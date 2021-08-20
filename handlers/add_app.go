package handlers

import (
	"fmt"
	"google-play-review-bot/utils"
	"io/ioutil"
	"log"

	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

type NewAppHandler struct {
}

var _ Handler = NewAppHandler{}

func (NewAppHandler) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/newapp") {
		return false
	}

	if !ctx.Update.Message.Chat.IsPrivate() {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Adding new app allowed only in private chats.")
		return true
	}

	if ctx.ChangeChatStateOrAnswerDefault(ChatStateWaitForOS) {
		message := tgbotapi.NewMessage(ctx.ChatId(), "Choose your os")
		rows := [][]tgbotapi.InlineKeyboardButton{
			{tgbotapi.NewInlineKeyboardButtonData("Android", "android"),
				tgbotapi.NewInlineKeyboardButtonData("iOS", "ios")},
		}
		message.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)

		ctx.Resp <- message
	}

	return true
}

func (NewAppHandler) Name() string {
	return "New App Handler"
}

type IosAndroidHandler struct {
}

var _ Handler = IosAndroidHandler{}

func (IosAndroidHandler) Handle(ctx Context) bool {
	if ok, _ := ctx.EnsureChatState(ChatStateWaitForOS); !ok {
		return false
	}

	os := ctx.Update.CallbackQuery.Data
	if os == "android" {
		ctx.ChangeChatStateWithNextState(ChatStateWaitForPackageName, ChatStateWaitForKey)
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Specify package name")
	} else if os == "ios" {
		ctx.ChangeChatState(ChatStateWaitForPackageName)
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Specify app id")
	} else {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Invalid OS")
		return true
	}

	ctx.SaveOS(os)

	return true
}

func (IosAndroidHandler) Name() string {
	return "IosAndroidHandler"
}

type PackageNameReceiver struct {
}

var _ Handler = PackageNameReceiver{}

func (PackageNameReceiver) Handle(ctx Context) bool {
	stateOk, chat := ctx.EnsureChatState(ChatStateWaitForPackageName)
	if !stateOk {
		return false
	}

	packageName := ctx.Update.Message.Text
	err := ctx.SavePackageName(packageName)

	if err != nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), fmt.Sprintf(
			"You already have app with packageName/appId = %s in this chat.\n Please provide another packageName or /reset",
			packageName))
	} else {
		nextState := int(chat.CustomData.(int32))
		if nextState != ChatStateNone {
			ctx.ChangeChatState(ChatStateWaitForKey)
			ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Please send json key")
		} else {
			ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Saved")
			err = ctx.ChangeChatState(ChatStateNone)
			utils.PanicOnError(err)

			ctx.AppChanges <- 1
		}
	}

	return true
}

func (PackageNameReceiver) Name() string {
	return "PackageNameReceiver"
}

type KeyReceiver struct {
}

var _ Handler = KeyReceiver{}

func (KeyReceiver) Handle(ctx Context) bool {
	if ok, _ := ctx.EnsureChatState(ChatStateWaitForKey); !ok {
		return false
	}

	fileId := ctx.Update.Message.Document.FileID
	reader, err := ctx.downloadFile(fileId)
	if err != nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Error downloading file: "+err.Error())
		return true
	}

	buf, err := ioutil.ReadAll(reader)
	log.Printf("Read %d bytes", len(buf))
	if err != nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Error downloading file: "+err.Error())
		return true
	}

	ctx.SetKeyFile(buf)
	err = ctx.ChangeChatState(ChatStateNone)
	utils.PanicOnError(err)

	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "File saved")
	ctx.AppChanges <- 1

	return true
}

func (KeyReceiver) Name() string {
	return "KeyReceiver"
}
