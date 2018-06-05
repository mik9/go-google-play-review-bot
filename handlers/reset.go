package handlers

import (
	"gopkg.in/telegram-bot-api.v4"
)

type Reset struct {
	Handler
}

func (Reset) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/reset") {
		return false
	}

	err := ctx.ChangeChatState(ChatStateNone)

	if err != nil {
		panic(err)
	}
	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "State reset")
	return true
}

func (Reset) Name() string {
	return "Reset"
}