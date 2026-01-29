package main

import (
	"fmt"
	"log"
	"strconv"
	"time"

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

	// Subscribe to pause exchange
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

	// Subscribe to army_moves exchange
	if err = pubsub.SubscribeJSON(
		connection,
		routing.ExchangePerilTopic,
		routing.ArmyMovesPrefix+"."+username,
		routing.ArmyMovesPrefix+".*",
		pubsub.QueueTypeTransient,
		handlerMove(gamestate, publishCh),
	); err != nil {
		log.Fatal(err)
	}

	// Subscribe to war exchange
	if err = pubsub.SubscribeJSON(
		connection,
		routing.ExchangePerilTopic,
		routing.WarRecognitionsPrefix,
		routing.WarRecognitionsPrefix+".*",
		pubsub.QueueTypeDurable,
		handlerWar(gamestate, publishCh),
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
			if len(input) < 2 {
				fmt.Println("Usage: spam <number>")
				continue
			}
			num, err := strconv.Atoi(input[1])
			if err != nil {
				fmt.Println("The provided argument is not an integer")
				continue
			}
			for range num {
				maliciousLog := gamelogic.GetMaliciousLog()
				if err := publishGameLog(
					publishCh,
					routing.GameLog{
						CurrentTime: time.Now(),
						Message:     maliciousLog,
						Username:    username,
					},
				); err != nil {
					fmt.Println("Couldn't publish spam log")
					continue
				}
			}
		case "quit":
			gamelogic.PrintQuit()
			break L
		default:
			fmt.Println("Unknown command: " + input[0])
			gamelogic.PrintClientHelp()
		}
	}
}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckType {
	defer fmt.Print("> ")
	return func(ps routing.PlayingState) pubsub.AckType {
		gs.HandlePause(ps)
		return pubsub.AckTypeAck
	}
}

func handlerMove(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.ArmyMove) pubsub.AckType {
	defer fmt.Print("> ")
	return func(mv gamelogic.ArmyMove) pubsub.AckType {
		switch gs.HandleMove(mv) {
		case gamelogic.MoveOutcomeMakeWar:
			if err := pubsub.PublishJSON(
				ch,
				routing.ExchangePerilTopic,
				routing.WarRecognitionsPrefix+"."+gs.Player.Username,
				gamelogic.RecognitionOfWar{
					Attacker: mv.Player,
					Defender: gs.GetPlayerSnap(),
				},
			); err != nil {
				return pubsub.AckTypeNackRequeue
			}
			return pubsub.AckTypeAck
		case gamelogic.MoveOutComeSafe:
			return pubsub.AckTypeAck
		}
		return pubsub.AckTypeNackDiscard
	}
}

func handlerWar(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.RecognitionOfWar) pubsub.AckType {
	defer fmt.Print("> ")
	return func(rw gamelogic.RecognitionOfWar) pubsub.AckType {
		outcome, winner, loser := gs.HandleWar(rw)
		switch outcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.AckTypeNackRequeue

		case gamelogic.WarOutcomeNoUnits:
			return pubsub.AckTypeNackDiscard

		case gamelogic.WarOutcomeOpponentWon, gamelogic.WarOutcomeYouWon:
			if err := publishGameLog(
				ch,
				routing.GameLog{
					CurrentTime: time.Now(),
					Message:     fmt.Sprintf("%s won a war against %s", winner, loser),
					Username:    gs.Player.Username,
				},
			); err != nil {
				return pubsub.AckTypeNackRequeue
			}
			return pubsub.AckTypeAck

		case gamelogic.WarOutcomeDraw:
			if err := publishGameLog(
				ch,
				routing.GameLog{
					CurrentTime: time.Now(),
					Message:     fmt.Sprintf("A war between %s and %s resulted in a draw", winner, loser),
					Username:    gs.Player.Username,
				},
			); err != nil {
				return pubsub.AckTypeNackRequeue
			}
			return pubsub.AckTypeAck

		default:
			fmt.Println("unknown war outcome")
			return pubsub.AckTypeNackDiscard
		}
	}
}

func publishGameLog(ch *amqp.Channel, gl routing.GameLog) error {
	if err := pubsub.PublishGob(
		ch,
		routing.ExchangePerilTopic,
		routing.GameLogSlug+"."+gl.Username,
		gl,
	); err != nil {
		return err
	}
	return nil
}
