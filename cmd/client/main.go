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
	publishCh, err := connection.Channel()
	if err != nil {
		log.Fatalf("Error creating publish channel: %v", err)
	}
	defer connection.Close()
	fmt.Println("Successfully connected to RabbitMQ!")

	username, err := gamelogic.ClientWelcome()
	if err != nil {
		log.Fatal(err)
	}

	gamestate := gamelogic.NewGameState(username)
	if err = pubsub.SubscribeJSON(
		connection,
		routing.ExchangePerilDirect,
		"pause."+username,
		"pause",
		pubsub.QueueTypeTransient,
		handlerPause(gamestate),
	); err != nil {
		log.Fatal(err)
	}

	if err = pubsub.SubscribeJSON(
		connection,
		routing.ExchangePerilTopic,
		routing.ArmyMovesPrefix+"."+username,
		routing.ArmyMovesPrefix+".*",
		pubsub.QueueTypeTransient,
		handlerMove(gamestate),
	); err != nil {
		log.Fatal(err)
	}

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
			move, err := gamestate.CommandMove(words)
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			if err = pubsub.PublishJSON(
				publishCh,
				routing.ExchangePerilTopic,
				routing.ArmyMovesPrefix+"."+username,
				move,
			); err != nil {
				fmt.Println(err.Error())
				continue
			}
			fmt.Println("Move was published successfully")
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

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) {
	defer fmt.Print("> ")
	return gs.HandlePause
}

func handlerMove(gs *gamelogic.GameState) func(gamelogic.ArmyMove) {
	defer fmt.Print("> ")
	return func(mv gamelogic.ArmyMove) {
		gs.HandleMove(mv)
	}
}
