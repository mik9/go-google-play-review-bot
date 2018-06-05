package handlers

import (
	"strings"
	"github.com/globalsign/mgo/bson"
	"gopkg.in/telegram-bot-api.v4"
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
		id := bson.ObjectIdHex(chunks[1])

		ctx.BindAppToChatId(id, ctx.ChatId())
	} else {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Hello!")
	}

	return true
}

func (StartHandler) Name() string {
	return "StartHandler"
}
