package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	botToken := os.Getenv("SLACK_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("SLACK_BOT_TOKEN environment variable is required")
	}

	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		log.Fatal("SLACK_APP_TOKEN environment variable is required")
	}

	channelID := os.Getenv("SLACK_CHANNEL_ID")
	if channelID == "" {
		log.Fatal("SLACK_CHANNEL_ID environment variable is required")
	}

	message := os.Getenv("MESSAGE")
	if message == "" {
		log.Fatal("MESSAGE environment variable is required")
	}

	timeoutStr := os.Getenv("TIMEOUT_SECONDS")
	if timeoutStr == "" {
		log.Fatal("TIMEOUT_SECONDS environment variable is required")
	}
	timeoutSec, err := strconv.Atoi(timeoutStr)
	if err != nil {
		log.Fatalf("Invalid TIMEOUT_SECONDS: %v", err)
	}

	maxDelayStr := os.Getenv("MAX_DELAY_SECONDS")
	if maxDelayStr == "" {
		log.Fatal("MAX_DELAY_SECONDS environment variable is required")
	}
	maxDelaySec, err := strconv.Atoi(maxDelayStr)
	if err != nil {
		log.Fatalf("Invalid MAX_DELAY_SECONDS: %v", err)
	}

	buttonLabel := os.Getenv("DELETE_BUTTON_LABEL")
	if buttonLabel == "" {
		log.Fatal("DELETE_BUTTON_LABEL environment variable is required")
	}

	timeoutMessage := os.Getenv("TIMEOUT_MESSAGE")
	if timeoutMessage == "" {
		log.Fatal("TIMEOUT_MESSAGE environment variable is required")
	}

	allowedUserIDsStr := os.Getenv("ALLOWED_USER_IDS")
	if allowedUserIDsStr == "" {
		log.Fatal("ALLOWED_USER_IDS environment variable is required")
	}
	allowedUserIDs := strings.Split(allowedUserIDsStr, ",")

	unauthorizedMessage := os.Getenv("UNAUTHORIZED_MESSAGE")
	if unauthorizedMessage == "" {
		log.Fatal("UNAUTHORIZED_MESSAGE environment variable is required")
	}

	client := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)

	socketClient := socketmode.New(client)

	// Post message with button
	deleteButton := slack.NewButtonBlockElement(
		"delete_message",
		"delete",
		slack.NewTextBlockObject("plain_text", buttonLabel, false, false),
	)
	deleteButton.Style = "danger"

	actionBlock := slack.NewActionBlock(
		"actions",
		deleteButton,
	)

	textBlock := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", message, false, false),
		nil,
		nil,
	)

	timeoutTextBlock := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", timeoutMessage, false, false),
		nil,
		nil,
	)

	// Random delay before posting
	delaySec := rand.IntN(maxDelaySec + 1)
	fmt.Printf("Waiting %d seconds before posting...\n", delaySec)
	time.Sleep(time.Duration(delaySec) * time.Second)

	_, timestamp, err := client.PostMessage(
		channelID,
		slack.MsgOptionBlocks(textBlock, actionBlock),
	)
	if err != nil {
		log.Fatalf("Failed to post message: %v", err)
	}

	fmt.Printf("Message posted successfully at %s\n", timestamp)

	// Timeout handler
	go func() {
		time.Sleep(time.Duration(timeoutSec) * time.Second)
		// Remove button by updating message with text only
		client.UpdateMessage(channelID, timestamp, slack.MsgOptionBlocks(timeoutTextBlock))
		fmt.Println("Timeout reached, exiting")
		os.Exit(0)
	}()

	// Handle events via Socket Mode
	go func() {
		for evt := range socketClient.Events {
			switch evt.Type {
			case socketmode.EventTypeDisconnect:
				fmt.Println("Disconnected from Slack, exiting")
				os.Exit(1)

			case socketmode.EventTypeInteractive:
				callback, ok := evt.Data.(slack.InteractionCallback)
				if !ok {
					continue
				}

				for _, action := range callback.ActionCallback.BlockActions {
					if action.ActionID == "delete_message" {
						if !slices.Contains(allowedUserIDs, callback.User.ID) {
							log.Printf("User %s is not allowed to delete", callback.User.ID)
							client.PostEphemeral(
								callback.Channel.ID,
								callback.User.ID,
								slack.MsgOptionText(unauthorizedMessage, false),
							)
							socketClient.Ack(*evt.Request)
							continue
						}

						_, _, err := client.DeleteMessage(
							callback.Channel.ID,
							callback.Message.Timestamp,
						)
						socketClient.Ack(*evt.Request)
						if err != nil {
							log.Printf("Failed to delete message: %v", err)
							continue
						}
						fmt.Println("Message deleted successfully")
						os.Exit(0)
					}
				}

				socketClient.Ack(*evt.Request)
			}
		}
		// Event channel closed unexpectedly
		fmt.Println("Event channel closed, exiting")
		os.Exit(1)
	}()

	if err := socketClient.Run(); err != nil {
		log.Fatalf("Socket mode error: %v", err)
	}
}
