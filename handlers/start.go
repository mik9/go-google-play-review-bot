package handlers

import (
	"google-play-review-bot/utils"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

type StartHandler struct {
	Handler
}

func (StartHandler) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/start") {
		return false
	}

	chunks := strings.Split(ctx.Update.Message.Text, " ")
	if len(chunks) > 1 {
		id, err := primitive.ObjectIDFromHex(chunks[1])
		utils.PanicOnError(err)

		ctx.BindAppToChatId(id, ctx.ChatId())
	} else {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Hello!")
	}

	return true
}

func (StartHandler) Name() string {
	return "StartHandler"
}
