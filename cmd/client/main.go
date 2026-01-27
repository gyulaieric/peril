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
	fmt.Println("Starting Peril client...")

	const connectionString string = "amqp://guest:guest@localhost:5672/"
	connection, err := amqp.Dial(connectionString)
	if err != nil {
		log.Fatalf("Error connecting to RabbitMQ: %v", err)
	}
	defer connection.Close()
	fmt.Println("Successfully connected to RabbitMQ!")

	username, err := gamelogic.ClientWelcome()
	if err != nil {
		log.Fatal(err)
	}

	_, _, err = pubsub.DeclareAndBind(
		connection,
		routing.ExchangePerilDirect,
		routing.PauseKey+"."+username,
		routing.PauseKey,
		pubsub.QueueTypeTransient,
	)
	if err != nil {
		log.Fatalf("Error binding queue: %v", err)
	}

	gamestate := gamelogic.NewGameState(username)

L:
	for {
		input := gamelogic.GetInput()
		if len(input) == 0 {
			continue
		}

		words := input[0:]

		switch input[0] {
		case "spawn":
			if err = gamestate.CommandSpawn(words); err != nil {
				fmt.Println(err.Error())
			}
		case "move":
			if _, err := gamestate.CommandMove(words); err != nil {
				fmt.Println(err.Error())
			}
		case "status":
			gamestate.CommandStatus()
		case "help":
			gamelogic.PrintClientHelp()
		case "spam":
			fmt.Println("Spamming not allowed yet!")
		case "quit":
			gamelogic.PrintQuit()
			break L
		default:
			fmt.Println("Unknown command: " + input[0])
			gamelogic.PrintClientHelp()
		}
	}
}
