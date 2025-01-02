package main

import (
	"flag"
	"fmt"
	"github.com/mymmrac/telego"
	"github.com/redis/go-redis/v9"
	"log"
	"os"
	"strconv"
	"time"
)

// RedisAuthorPrefix is a prefix for Redis keys with author IDs. It's used to store author IDs for future responses.
const RedisAuthorPrefix = "hd_msg"

// RedisCacheExpiration is a time duration for Redis keys with author IDs.
const RedisCacheExpiration = time.Hour * 24 * 7

// StartingMessage is a message that bot sends to users when they start a chat.
const StartingMessage = "👋 Hi! I'm a bot that forwards your messages to the support. Just type your message and I'll take care of the rest."

func main() {
	debug := flag.Bool("debug", false, "Run in debug mode (more logs)")
	flag.Parse()

	helpdeskChatId, err := strconv.ParseInt(os.Getenv("HELPDESK_CHAT_ID"), 10, 64)

	// Logger configuration (more or less logs)
	var logger telego.BotOption
	if *debug {
		logger = telego.WithDefaultDebugLogger()
	} else {
		logger = telego.WithDiscardLogger()
	}

	bot, err := telego.NewBot(os.Getenv("TOKEN"), logger)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	db := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_URL"),
	})

	botUser, err := bot.GetMe()
	if err != nil {
		fmt.Println("Error:", err)
	}

	fmt.Println("Bot user: ", botUser.Username)
	helpdeskChat := telego.ChatID{
		ID: helpdeskChatId,
	}

	updates, _ := bot.UpdatesViaLongPolling(nil)
	defer bot.StopLongPolling()

	for update := range updates {
		if update.Message == nil {
			continue
		}
		if update.Message.From.IsBot {
			continue
		}

		chatId, fromId := update.Message.Chat.ID, update.Message.From.ID
		chatIdObj := telego.ChatID{
			ID: chatId,
		}

		// Template for future response
		message := &telego.SendMessageParams{
			ChatID:           chatIdObj,
			ReplyToMessageID: update.Message.MessageID,
		}

		// You can respond only in special chat
		if chatId == helpdeskChatId {
			if update.Message.ReplyToMessage == nil {
				continue
			} else {
				forwardFrom := update.Message.ReplyToMessage.ForwardFromMessageID
				replyTo := 0
				var authorId int64

				if forwardFrom <= 0 {
					authorId, err = strconv.ParseInt(update.Message.ReplyToMessage.Text, 10, 64)
				} else {
					replyTo = update.Message.ReplyToMessage.MessageID
					questionAuthor := db.Get(update.Context(), RedisAuthorPrefix+strconv.Itoa(forwardFrom))
					authorId, err = questionAuthor.Int64()
				}

				if err != nil {
					message.ChatID = chatIdObj
					message.Text = "Author not found: " + err.Error()
				} else {
					copyResult, err := bot.CopyMessage(&telego.CopyMessageParams{
						ChatID: telego.ChatID{
							ID: authorId,
						},
						FromChatID:       chatIdObj,
						MessageID:        update.Message.MessageID,
						ReplyToMessageID: replyTo,
					})
					message.ChatID = helpdeskChat

					if err != nil {
						message.Text = "Error forwarding from " + chatIdObj.String() + ": " + err.Error()
					} else {
						message.Text = "Reply sent (" + strconv.Itoa(copyResult.MessageID) + ")"
					}
				}
			}
		} else if chatId < 0 {
			// Ignore other chats
			continue
		} else if update.Message.Text == "/start" {
			message.Text = StartingMessage
		} else {
			set := db.Set(update.Context(), RedisAuthorPrefix+strconv.Itoa(update.Message.MessageID), fromId, RedisCacheExpiration)
			if err := set.Err(); err != nil {
				log.Println(err)
				message.Text = "Error saving author ID"
			} else {
				message.ChatID = helpdeskChat
				message.ReplyToMessageID = 0
				_, err := bot.ForwardMessage(&telego.ForwardMessageParams{
					ChatID:     helpdeskChat,
					FromChatID: chatIdObj,
					MessageID:  update.Message.MessageID,
				})

				// Report about errors
				if err != nil {
					message.Text = "Error forwarding from " + chatIdObj.String() + ": " + err.Error()
				} else {
					message.Text = strconv.FormatInt(chatId, 10)
				}
			}
		}

		// Don't send empty messages
		if message.Text == "" {
			continue
		}

		_, _ = bot.SendMessage(message)
	}
}
