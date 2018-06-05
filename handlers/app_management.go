package handlers

import (
	"google-play-review-bot/collections"
	"github.com/globalsign/mgo/bson"
	"gopkg.in/telegram-bot-api.v4"
	"fmt"
	"log"
	"time"
	"strings"
)

type AppList struct {
	Handler
}

func (AppList) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/apps") {
		return false
	}

	var apps []struct{ PackageName string }
	ctx.Db.C(collections.APPS).Find(bson.M{
		"userid": ctx.UserId(),
	}).Select(bson.M{"packagename": 1}).All(&apps)

	var resp string
	if len(apps) == 0 {
		resp = "You have no configured apps yet. /newapp ?"
	} else {
		resp = "You have next apps:\n"
		for _, v := range apps {
			resp = resp + v.PackageName + "\n"
		}
	}

	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), resp)

	return true
}

func (AppList) Name() string {
	return "AppList"
}

type ChangeLanguage struct {
	Handler
}

func (ChangeLanguage) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/changelanguage") {
		return false
	}

	chattable := makeAppChooser(ctx)
	if chattable == nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "No apps to change")
		return true
	}

	if !ctx.ChangeChatStateWithNextStateOrAnswerDefault(ChatStateWaitForApp, ChatStateWaitForLanguage) {
		return false
	}

	if chattable != nil {
		ctx.Resp <- *chattable
	}

	return true
}

func (ChangeLanguage) Name() string {
	return "ChangeLanguage"
}

func makeAppChooser(ctx Context) *tgbotapi.Chattable {
	var apps []Application
	ctx.Db.C(collections.APPS).Find(bson.M{
		"userid": ctx.UserId(),
	}).All(&apps)
	
	if len(apps) == 0 {
		return nil
	}

	var message = tgbotapi.NewMessage(ctx.ChatId(), "Please choose app")
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, app := range apps {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(app.GetName(), app.ID.Hex()),
		})
	}
	log.Printf("k: %d %d %+v", len(apps), len(rows), rows)
	message.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)

	chattable := tgbotapi.Chattable(message)

	return &chattable
}

type ChangeLanguageReceiver struct {
	Handler
}

func (ChangeLanguageReceiver) Handle(ctx Context) bool {
	stateOk, chat := ctx.EnsureChatState(ChatStateWaitForLanguage)
	if !stateOk {
		return false
	}

	language := ctx.Update.Message.Text

	err := ctx.Db.C(collections.APPS).UpdateId(chat.CustomData, bson.M{
		"$set": bson.M{
			"translatelanguage": language,
			"lastreview":        time.Time{},
		},
	})
	if err != nil {
		panic(err)
	}

	ctx.ChangeChatStateWithNextState(ChatStateNone, ChatStateNone)

	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Language changed")
	ctx.AppChanges <- 1

	return true
}

func (ChangeLanguageReceiver) Name() string {
	return "ChangeLanguageReceiver"
}

type ChooseAppReceiver struct {
	Handler
}

func (ChooseAppReceiver) Handle(ctx Context) bool {
	stateOk, chat := ctx.EnsureChatState(ChatStateWaitForApp)
	if !stateOk || ctx.Update.CallbackQuery == nil {
		return false
	}

	id := ctx.Update.CallbackQuery.Data
	nextState := chat.CustomData.(int)

	if nextState >= 0 {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), fmt.Sprintf("Please provide %s", ChatStateToWaitingString(nextState)))
	}

	var stateCall int
	if nextState < 0 {
		stateCall = nextState
		nextState = ChatStateNone
	}

	err := ctx.Db.C(collections.CHAT).Update(bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	}, bson.M{
		"$set": bson.M{
			"customdata": bson.ObjectIdHex(id),
			"state":      nextState,
		},
	})
	if err != nil {
		panic(err)
	}
	if stateCall != 0 {
		ChatStateCall(stateCall, ctx)
	}

	return true
}

func (ChooseAppReceiver) Name() string {
	return "ChooseAppReceiver"
}

type ChangeAppName struct {
	Handler
}

func (ChangeAppName) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/changeappname") {
		return false
	}

	chattable := makeAppChooser(ctx)
	if chattable == nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "No apps to change")
		return true
	}

	if !ctx.ChangeChatStateWithNextStateOrAnswerDefault(ChatStateWaitForApp, ChatStateWaitForAppName) {
		return false
	}

	if chattable != nil {
		ctx.Resp <- *chattable
	}

	return true
}

func (ChangeAppName) Name() string {
	return "ChangeAppName"
}

type ChangeAppNameReceiver struct {
	Handler
}

func (ChangeAppNameReceiver) Handle(ctx Context) bool {
	stateOk, chat := ctx.EnsureChatState(ChatStateWaitForAppName)
	if !stateOk {
		return false
	}

	name := ctx.Update.Message.Text

	err := ctx.Db.C(collections.APPS).UpdateId(chat.CustomData, bson.M{
		"$set": bson.M{
			"name":       name,
			"lastreview": time.Time{},
		},
	})
	if err != nil {
		panic(err)
	}

	ctx.ChangeChatStateWithNextState(ChatStateNone, ChatStateNone)

	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "App name changed")
	ctx.AppChanges <- 1

	return true
}

func (ChangeAppNameReceiver) Name() string {
	return "ChangeAppNameReceiver"
}

type ChangeGroup struct {
	Handler
}

func (ChangeGroup) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/changegroup") {
		return false
	}

	chattable := makeAppChooser(ctx)
	if chattable == nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "No apps to change")
		return true
	}

	if !ctx.ChangeChatStateWithNextStateOrAnswerDefault(ChatStateWaitForApp, ChatStateCallChangeGroupReceiver) {
		return false
	}

	if chattable != nil {
		ctx.Resp <- *chattable
	}

	return true
}

func (ChangeGroup) Name() string {
	return "ChangeGroup"
}

type ChangeGroupReceiver struct {
	Handler
}

func (ChangeGroupReceiver) Handle(ctx Context) bool {
	log.Printf("ChangeGroupReceiver")
	var chat Chat
	chatSelector := bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	}
	ctx.Db.C(collections.CHAT).Find(chatSelector).One(&chat)

	id := chat.CustomData.(bson.ObjectId)
	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Great, check private chat for further instructions.")
	ctx.Resp <- tgbotapi.NewMessage(int64(ctx.UserId()),
		"Use next lint to add me to desired group: https://telegram.me/google_play_review_bot?startgroup="+id.Hex()+"\n Or leave it here: /private_"+id.Hex())

	ctx.Db.C(collections.APPS).UpdateId(id, bson.M{
		"$unset": bson.M{
			"chatid": 1,
		},
	})
	ctx.ChangeChatState(ChatStateNone)

	ctx.AppChanges <- 1

	return true
}

func (ChangeGroupReceiver) Name() string {
	return "ChangeGroup"
}

type ChangeGroupPrivateReceiver struct {
	Handler
}

func (ChangeGroupPrivateReceiver) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/private") {
		return false
	}

	appId := strings.Split(ctx.Update.Message.Text, "_")[1]

	ctx.BindAppToChatId(bson.ObjectIdHex(appId), int64(ctx.UserId()))

	return true
}

func (ChangeGroupPrivateReceiver) Name() string {
	return "ChangeGroupPrivateReceiver"
}