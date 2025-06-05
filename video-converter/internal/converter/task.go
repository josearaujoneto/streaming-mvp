package converter

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"video-converter/pkg/rabbitmq"

	"github.com/streadway/amqp"
)

type VideoConverter struct {
	rabbitClient *rabbitmq.RabbitClient
	db           *sql.DB
	rootPath     string
}

type VideoTask struct {
	VideoID int    `json:"video_id"`
	Path    string `json:"path"`
}

func NewVideoConverter(rabbitClient *rabbitmq.RabbitClient, db *sql.DB, rootPath string) *VideoConverter {
	return &VideoConverter{
		rabbitClient: rabbitClient,
		db:           db,
		rootPath:     rootPath,
	}
}

func (vc *VideoConverter) HandleMessage(ctx context.Context, d amqp.Delivery, conversionExch, confirmationKey, confirmationQueue string) {
	var task VideoTask

	if err := json.Unmarshal(d.Body, &task); err != nil {
		vc.logError(task, "Failed to deserialize message", err)
		d.Ack(false)
		return
	}

	if IsProcessed(vc.db, task.VideoID) {
		slog.Warn("Video already processed", slog.Int("video_id", task.VideoID))
		d.Ack(false)
		return
	}

	err := vc.processVideo(&task)
	if err != nil {
		vc.logError(task, "Error during video conversion", err)
		d.Ack(false)
		return
	}
	slog.Info("Video conversion processed", slog.Int("video_id", task.VideoID))

	err = MarkProcessed(vc.db, task.VideoID)
	if err != nil {
		vc.logError(task, "Failed to mark video as processed", err)
	}
	d.Ack(false)
	slog.Info("Video marked as processed", slog.Int("video_id", task.VideoID))

	confirmationMessage := []byte(fmt.Sprintf(`{"video_id": %d, "path":"%s"}`, task.VideoID, task.Path))
	err = vc.rabbitClient.PublishMessage(conversionExch, confirmationKey, confirmationQueue, confirmationMessage)
	if err != nil {
		slog.Error("Failed to publish confirmation message", slog.String("error", err.Error()))
	}
	slog.Info("Published confirmation message", slog.Int("video_id", task.VideoID))
}

func (vc *VideoConverter) processVideo(task *VideoTask) error {
	chunkPath := filepath.Join(vc.rootPath, fmt.Sprintf("%d", task.VideoID))
	mergedFile := filepath.Join(chunkPath, "merged.mp4")
	mpegDashPath := filepath.Join(chunkPath, "mpeg-dash")

	slog.Info("Merging chunks", slog.String("path", chunkPath))
	if err := vc.mergeChunks(chunkPath, mergedFile); err != nil {
		return fmt.Errorf("failed to merge chunks: %v", err)
	}

	if err := os.MkdirAll(mpegDashPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	ffmpegCmd := exec.Command(
		"ffmpeg", "-i", mergedFile,
		"-f", "dash",
		filepath.Join(mpegDashPath, "output.mpd"),
	)

	output, err := ffmpegCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to convert to MPEG-DASH: %v, output: %s", err, string(output))
	}
	slog.Info("Converted to MPEG-DASH", slog.String("path", mpegDashPath))

	if err := os.Remove(mergedFile); err != nil {
		slog.Warn("Failed to remove merged file", slog.String("file", mergedFile), slog.String("error", err.Error()))
	}
	slog.Info("Removed merged file", slog.String("file", mergedFile))

	return nil
}

func (vc *VideoConverter) extractNumber(fileName string) int {
	re := regexp.MustCompile(`\d+`)
	numStr := re.FindString(filepath.Base(fileName))
	num, _ := strconv.Atoi(numStr)
	return num
}

func (vc *VideoConverter) mergeChunks(inputDir, outputFile string) error {
	chunks, err := filepath.Glob(filepath.Join(inputDir, "*.chunk"))
	if err != nil {
		return fmt.Errorf("failed to find chunks: %v", err)
	}

	sort.Slice(chunks, func(i, j int) bool {
		return vc.extractNumber(chunks[i]) < vc.extractNumber(chunks[j])
	})

	output, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create merged file: %v", err)
	}
	defer output.Close()

	for _, chunk := range chunks {
		input, err := os.Open(chunk)
		if err != nil {
			return fmt.Errorf("failed to open chunk %s: %v", chunk, err)
		}

		_, err = output.ReadFrom(input)
		if err != nil {
			return fmt.Errorf("failed to write chunk %s to merged file: %v", chunk, err)
		}
		input.Close()
	}
	return nil
}

func (vc *VideoConverter) logError(task VideoTask, message string, err error) {
	errorData := map[string]interface{}{
		"video_id": task.VideoID,
		"error":    message,
		"details":  err.Error(),
		"time":     time.Now(),
	}

	serializedError, _ := json.Marshal(errorData)
	slog.Error("Processing error", slog.String("error_details", string(serializedError)))

	RegisterError(vc.db, errorData, err)
}
