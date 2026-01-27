package main

import (
	"fmt"
	"log"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril server...")

	const connectionString string = "amqp://guest:guest@localhost:5672/"
	connection, err := amqp.Dial(connectionString)
	if err != nil {
		log.Fatalf("Error connecting to RabbitMQ: %v", err)
	}
	defer connection.Close()
	fmt.Println("Successfully connected to RabbitMQ!")

	channel, err := connection.Channel()
	if err != nil {
		log.Fatalf("Error creating connection channel: %v", err)
	}

	gamelogic.PrintServerHelp()

L:
	for {
		input := gamelogic.GetInput()
		if len(input) == 0 {
			continue
		}

		switch input[0] {
		case "pause":
			fmt.Println("Sending pause message...")
			if err := sendPauseMessage(channel, true); err != nil {
				log.Fatalf("Error sending message: %v", err)
			}

		case "resume":
			fmt.Println("Sending resume message...")
			if err := sendPauseMessage(channel, false); err != nil {
				log.Fatalf("Error sending message: %v", err)
			}

		case "help":
			gamelogic.PrintServerHelp()

		case "quit":
			fmt.Println("Quitting...")
			break L

		default:
			fmt.Println("Unknown command: " + input[0])
			gamelogic.PrintServerHelp()
		}
	}
}

func sendPauseMessage(ch *amqp.Channel, paused bool) error {
	if err := pubsub.PublishJSON(
		ch, routing.ExchangePerilDirect,
		routing.PauseKey,
		routing.PlayingState{IsPaused: paused},
	); err != nil {
		return err
	}

	return nil
}
