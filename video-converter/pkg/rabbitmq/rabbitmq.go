package rabbitmq

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/streadway/amqp"
)

type RabbitClientInterface interface {
	ConsumeMessages(exchange, routingKey, queueName string) (<-chan amqp.Delivery, error)
	PublishMessage(exchange, routingKey, queueName string, message []byte) error // Alterado para aceitar o nome da fila
	Close() error
	IsClosed() bool
}

type RabbitClient struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	url     string
}

func newConnection(url string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to RabbitMQ: %v", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to open channel: %v", err)
	}

	return conn, channel, nil
}

func NewRabbitClient(ctx context.Context, connectionURL string) (*RabbitClient, error) {
	conn, channel, err := newConnection(connectionURL)
	if err != nil {
		return nil, err
	}

	return &RabbitClient{
		conn:    conn,
		channel: channel,
		url:     connectionURL,
	}, nil
}

func (client *RabbitClient) ConsumeMessages(exchange, routingKey, queueName string) (<-chan amqp.Delivery, error) {
	err := client.channel.ExchangeDeclare(
		exchange, "direct", true, true, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to declare exchange: %v", err)
	}

	queue, err := client.channel.QueueDeclare(
		queueName, true, true, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue: %v", err)
	}

	err = client.channel.QueueBind(queue.Name, routingKey, exchange, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to bind queue: %v", err)
	}

	msgs, err := client.channel.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to consume messages: %v", err)
	}

	return msgs, nil
}

func (client *RabbitClient) PublishMessage(exchange, routingKey, queueName string, message []byte) error {
	err := client.channel.ExchangeDeclare(
		exchange, "direct", true, true, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare exchange: %v", err)
	}

	_, err = client.channel.QueueDeclare(
		queueName, true, true, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %v", err)
	}

	err = client.channel.QueueBind(queueName, routingKey, exchange, false, nil)
	if err != nil {
		return fmt.Errorf("failed to bind queue to exchange: %v", err)
	}

	err = client.channel.Publish(
		exchange, routingKey, false, false, amqp.Publishing{
			ContentType: "application/json",
			Body:        message,
		})
	if err != nil {
		return fmt.Errorf("failed to publish message: %v", err)
	}
	return nil
}

func (client *RabbitClient) IsClosed() bool {
	return client.conn.IsClosed()
}

func (client *RabbitClient) Close() error {
	err := client.channel.Close()
	if err != nil {
		return fmt.Errorf("failed to close channel: %v", err)
	}
	err = client.conn.Close()
	if err != nil {
		return fmt.Errorf("failed to close connection: %v", err)
	}
	return nil
}

func (client *RabbitClient) Reconnect(ctx context.Context) error {
	var err error
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled while trying to reconnect")
		default:
			slog.Info("Attempting to reconnect to RabbitMQ...")
			client.conn, client.channel, err = newConnection(client.url)
			if err == nil {
				slog.Info("Reconnected to RabbitMQ successfully")
				return nil
			}
			slog.Error("Failed to reconnect to RabbitMQ", slog.String("error", err.Error()))
			time.Sleep(5 * time.Second)
		}
	}
}
