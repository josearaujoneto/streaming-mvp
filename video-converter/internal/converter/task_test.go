//go:build e2e

package converter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"imersaofc/internal/converter"
	"imersaofc/pkg/rabbitmq"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	videoID               = 1
	videoPath             = "../../mediatest/media/uploads/1"
	expectedPath          = "../../mediatest/media/uploads/1/mpeg-dash"
	outputFile            = "output.mpd"
	conversionKey         = "conversion"
	finishConversionKey   = "finish-conversion"
	conversionQueue       = "video_conversion_queue"
	finishConversionQueue = "video_confirmation_queue"
	exchangeName          = "conversion_exchange"
)

func TestTaskProcessing(t *testing.T) {
	ctx := context.Background()

	postgresContainer, db, err := setupPostgresContainer(ctx)
	assert.NoError(t, err)
	defer postgresContainer.Terminate(ctx)
	defer db.Close()

	rabbitmqC, rabbitMQURL, err := startRabbitMQContainer(ctx)
	assert.NoError(t, err, "Failed to start RabbitMQ container")
	defer rabbitmqC.Terminate(ctx)

	rabbitClient, err := rabbitmq.NewRabbitClient(ctx, rabbitMQURL)
	assert.NoError(t, err, "Failed to connect to RabbitMQ")
	defer rabbitClient.Close()

	rootPath := "../../mediatest/media/uploads"
	videoConverter := converter.NewVideoConverter(rabbitClient, db, rootPath)

	videoTask := converter.VideoTask{
		VideoID: videoID,
		Path:    videoPath,
	}

	taskMessage, err := json.Marshal(videoTask)
	assert.NoError(t, err)

	msgs, err := rabbitClient.ConsumeMessages(exchangeName, conversionKey, conversionQueue)
	assert.NoError(t, err, "Failed to consume messages from video conversion queue")

	err = rabbitClient.PublishMessage(exchangeName, conversionKey, conversionQueue, taskMessage)
	assert.NoError(t, err, "Failed to publish message to conversion queue")

	videoConverter.HandleMessage(ctx, <-msgs)

	confirmationTask := converter.VideoTask{
		VideoID: videoID,
		Path:    expectedPath,
	}
	confirmationMessage, _ := json.Marshal(confirmationTask)
	err = rabbitClient.PublishMessage(exchangeName, finishConversionKey, finishConversionQueue, confirmationMessage)
	assert.NoError(t, err, "Failed to publish message to confirmation queue")

	strVideoID := fmt.Sprintf("%d", videoID)
	outputFilePath := filepath.Join(rootPath, strVideoID, "mpeg-dash", outputFile)
	_, err = os.Stat(outputFilePath)
	assert.NoError(t, err, "Output file was not created")

	confirmationMsgs, err := rabbitClient.ConsumeMessages(exchangeName, finishConversionKey, finishConversionQueue)
	assert.NoError(t, err, "Failed to consume confirmation messages")

	select {
	case msg := <-confirmationMsgs:
		var confirmation converter.VideoTask
		err = json.Unmarshal(msg.Body, &confirmation)
		fmt.Println(confirmation)
		assert.NoError(t, err)
		assert.Equal(t, videoTask.VideoID, confirmation.VideoID)
		assert.Equal(t, filepath.Join(videoTask.Path, "mpeg-dash"), confirmation.Path)
		msg.Ack(false)
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for the confirmation message")
	}

	var processed bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM processed_videos WHERE video_id = $1 AND status = 'success')", videoTask.VideoID).Scan(&processed)
	assert.NoError(t, err)
	assert.True(t, processed, "Video was not marked as processed in the database")

	err = os.RemoveAll(filepath.Join(rootPath, strVideoID, "mpeg-dash"))
	assert.NoError(t, err, "Failed to remove mpeg-dash folder")
}
