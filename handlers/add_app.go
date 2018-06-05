package handlers

import (
	"gopkg.in/telegram-bot-api.v4"
	"google-play-review-bot/collections"
	"github.com/globalsign/mgo/bson"
	"google-play-review-bot/utils"
	"fmt"
	"log"
	"io/ioutil"
)

type NewAppHandler struct {
	Handler
}

func (NewAppHandler) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/newapp") {
		return false
	}

	if !ctx.Update.Message.Chat.IsPrivate() {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Adding new app allowed only in private chats.")
		return true
	}

	if ctx.ChangeChatStateOrAnswerDefault(ChatStateWaitForPackageName) {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Specify package name")
	}

	return true
}

func (NewAppHandler) Name() string {
	return "New App Handler"
}

type PackageNameReceiver struct {
	Handler
}

func (PackageNameReceiver) Handle(ctx Context) bool {
	if  ok, _ := ctx.EnsureChatState(ChatStateWaitForPackageName); !ok {
		return false
	}

	packageName := ctx.Update.Message.Text

	err := ctx.Db.C(collections.APPS).Insert(bson.M{
		"packagename": packageName,
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
		"translatelanguage": "en",
	})

	utils.LogError(err)

	if err != nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), fmt.Sprintf(
			"You already have app with packageName = %s in this chat.\n Please provide another packageName or /reset",
			packageName))
	} else {
		ctx.ChangeChatState(ChatStateWaitForKey)
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Please send json key")
	}

	return true
}

func (PackageNameReceiver) Name() string {
	return "PackageNameReceiver"
}

type KeyReceiver struct {
	Handler
}

func (KeyReceiver) Handle(ctx Context) bool {
	if ok, _ := ctx.EnsureChatState(ChatStateWaitForKey); !ok {
		return false
	}
	fileId := ctx.Update.Message.Document.FileID
	reader, err := ctx.downloadFile(fileId)
	if err != nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Error downloading file: "+err.Error())
	}

	buf, err := ioutil.ReadAll(reader)
	log.Printf("Read %d bytes", len(buf))
	utils.LogError(err)

	ctx.SetKeyFile(buf)
	ctx.ChangeChatState(ChatStateNone)

	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "File saved")

	return true
}

func (KeyReceiver) Name() string {
	return "KeyReceiver"
}
