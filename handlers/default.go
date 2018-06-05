package handlers

import "gopkg.in/telegram-bot-api.v4"

type DefaultHandler struct {
	Handler
}

func (DefaultHandler) Handle(ctx Context) bool {
	update := ctx.Update
	if update.Message != nil {
		resp := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
		resp.ReplyToMessageID = update.Message.MessageID

		ctx.Resp <- resp
		return true
	}

	return false
}

func (DefaultHandler) Name() string {
	return "Default Handler"
}

type EditMessageConsumer struct {
}

func (EditMessageConsumer) Handle(ctx Context) bool {
	if ctx.Update.EditedMessage != nil {
		return true
	}

	return false
}

func (EditMessageConsumer) Name() string {
	return "EditMessageConsumer"
}


