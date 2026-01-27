package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type SimpleQueueType int

const (
	QueueTypeDurable SimpleQueueType = iota
	QueueTypeTransient
)

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("couldn't marshal json value: %v", err)
	}

	if err = ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        data,
	}); err != nil {
		return fmt.Errorf("couldn't publish message: %v", err)
	}
	return nil
}

func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
) (*amqp.Channel, amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	durable := queueType == 0
	autoDelete := queueType == 1
	exclusive := queueType == 1

	queue, err := ch.QueueDeclare(queueName, durable, autoDelete, exclusive, false, nil)
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	if err = ch.QueueBind(queueName, key, exchange, false, nil); err != nil {
		return nil, amqp.Queue{}, err
	}

	return ch, queue, nil
}

func SubscribeJSON[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T),
) error {
	ch, queue, err := DeclareAndBind(conn, exchange, queueName, key, queueType)
	if err != nil {
		return err
	}

	deliveryChan, err := ch.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for delivery := range deliveryChan {
			var message T
			json.Unmarshal(delivery.Body, &message)
			handler(message)
			if err := delivery.Ack(false); err != nil {
				fmt.Println(err.Error())
			}
		}
	}()

	return nil
}
