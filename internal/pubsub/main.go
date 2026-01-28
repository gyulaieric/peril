package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type SimpleQueueType int

const (
	QueueTypeDurable SimpleQueueType = iota
	QueueTypeTransient
)

type AckType int

const (
	AckTypeAck = iota
	AckTypeNackRequeue
	AckTypeNackDiscard
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

	queue, err := ch.QueueDeclare(queueName, durable, autoDelete, exclusive, false, amqp.Table{"x-dead-letter-exchange": "peril_dlx"})
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
	handler func(T) AckType,
) error {
	return subscribe(
		conn,
		exchange,
		queueName,
		key,
		queueType,
		handler,
		func(val []byte) (T, error) {
			var message T
			if err := json.Unmarshal(val, &message); err != nil {
				return message, err
			}
			return message, nil
		},
	)
}

func PublishGob[T any](ch *amqp.Channel, exchange, key string, val T) error {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(val); err != nil {
		return fmt.Errorf("couldn't encode gob: %v", err)
	}

	if err := ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/gob",
		Body:        buf.Bytes(),
	}); err != nil {
		return fmt.Errorf("couldn't publish message: %v", err)
	}
	return nil
}

func SubscribeGob[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) AckType,
) error {
	return subscribe(
		conn,
		exchange,
		queueName,
		key,
		queueType,
		handler,
		func(val []byte) (T, error) {
			buf := bytes.NewBuffer(val)
			decoder := gob.NewDecoder(buf)

			var message T
			if err := decoder.Decode(&message); err != nil {
				return message, err
			}
			return message, nil
		},
	)
}

func subscribe[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) AckType,
	unmarshaller func([]byte) (T, error),
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
			message, err := unmarshaller(delivery.Body)
			if err != nil {
				fmt.Printf("Error unmarshaling message: %v", err)
				continue
			}
			switch handler(message) {
			case AckTypeAck:
				delivery.Ack(false)
				fmt.Println("Ack")
			case AckTypeNackRequeue:
				delivery.Nack(false, true)
				fmt.Println("NackRequeue")
			case AckTypeNackDiscard:
				delivery.Nack(false, false)
				fmt.Println("NackDiscard")
			}
		}
	}()

	return nil
}
