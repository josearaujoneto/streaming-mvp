package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"video-converter/internal/converter"
	"video-converter/pkg/log"
	"video-converter/pkg/rabbitmq"

	_ "github.com/lib/pq"
	"github.com/streadway/amqp"
)

func connectPostgres() (*sql.DB, error) {
	user := getEnvOrDefault("POSTGRES_USER", "user")
	password := getEnvOrDefault("POSTGRES_PASSWORD", "password")
	dbname := getEnvOrDefault("POSTGRES_DB", "converter")
	host := getEnvOrDefault("POSTGRES_HOST", "host.docker.internal")
	sslmode := getEnvOrDefault("POSTGRES_SSL_MODE", "disable")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s host=%s sslmode=%s", user, password, dbname, host, sslmode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("Failed to connect to PostgreSQL", slog.String("error", err.Error()))
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		slog.Error("Failed to ping PostgreSQL", slog.String("error", err.Error()))
		return nil, err
	}

	slog.Info("Connected to PostgreSQL successfully")
	return db, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func main() {
	isDebug := getEnvOrDefault("DEBUG", "false") == "true"
	logger := log.NewLogger(isDebug)
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	db, err := connectPostgres()
	if err != nil {
		return
	}
	defer db.Close()

	rabbitMQURL := getEnvOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/")
	rabbitClient, err := rabbitmq.NewRabbitClient(ctx, rabbitMQURL)
	if err != nil {
		slog.Error("Failed to connect to RabbitMQ", slog.String("error", err.Error()))
		return
	}
	defer rabbitClient.Close()

	conversionExch := getEnvOrDefault("CONVERSION_EXCHANGE", "conversion_exchange")
	queueName := getEnvOrDefault("QUEUE_NAME", "video_conversion_queue")
	conversionKey := getEnvOrDefault("CONVERSION_KEY", "conversion")
	confirmationKey := getEnvOrDefault("CONFIRMATION_KEY", "finish-conversion")
	rootPath := getEnvOrDefault("VIDEO_ROOT_PATH", "/media/uploads")
	confirmationQueue := "video_confirmation_queue"

	videoConverter := converter.NewVideoConverter(rabbitClient, db, rootPath)

	msgs, err := rabbitClient.ConsumeMessages(conversionExch, conversionKey, queueName)
	if err != nil {
		slog.Error("Failed to consume messages", slog.String("error", err.Error()))
		return
	}

	var wg sync.WaitGroup
	go func() {
		for d := range msgs {
			wg.Add(1)
			go func(delivery amqp.Delivery) {
				defer wg.Done()
				videoConverter.HandleMessage(ctx, delivery, conversionExch, confirmationKey, confirmationQueue)

			}(d)
		}
	}()

	slog.Info("Waiting for messages from RabbitMQ")
	<-signalChan
	slog.Info("Shutdown signal received, finalizing processing...")

	cancel()

	wg.Wait()

	slog.Info("Processing completed, exiting...")
}
