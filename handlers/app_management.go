package handlers

import (
	"fmt"
	"google-play-review-bot/collections"
	"google-play-review-bot/utils"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

type AppList struct {
	Handler
}

func (AppList) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/apps") {
		return false
	}

	var apps []struct{ PackageName string }
	c, err := ctx.Store.DB().Collection(collections.APPS).Find(ctx.Store.Context, bson.M{
		"userid": ctx.UserId(),
	}, options.Find().SetProjection(bson.M{"packagename": 1}))

	utils.PanicOnError(err)

	err = c.All(ctx.Store.Context, &apps)
	utils.PanicOnError(err)

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
	c, err := ctx.Store.DB().Collection(collections.APPS).Find(ctx.Store.Context, bson.M{
		"userid": ctx.UserId(),
	})
	utils.PanicOnError(err)

	err = c.All(ctx.Store.Context, &apps)
	utils.PanicOnError(err)

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

	_, err := ctx.Store.DB().Collection(collections.APPS).UpdateOne(ctx.Store.Context, bson.M{"_id": chat.CustomData}, bson.M{
		"$set": bson.M{
			"translatelanguage": language,
			"lastreview":        time.Time{},
		},
		"$unset": bson.M{
			"lastreviewid": 1,
		},
	})
	utils.PanicOnError(err)

	err = ctx.ChangeChatStateWithNextState(ChatStateNone, ChatStateNone)
	utils.PanicOnError(err)

	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Language changed")
	ctx.AppChanges <- 1

	return true
}

func (ChangeLanguageReceiver) Name() string {
	return "ChangeLanguageReceiver"
}

type ChooseAppReceiver struct {
	Handler
	BotUserName string
}

func (c ChooseAppReceiver) Handle(ctx Context) bool {
	stateOk, chat := ctx.EnsureChatState(ChatStateWaitForApp)
	if !stateOk || ctx.Update.CallbackQuery == nil {
		return false
	}

	id := ctx.Update.CallbackQuery.Data
	nextState := int(chat.CustomData.(int32))

	if nextState >= 0 {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), fmt.Sprintf("Please provide %s", ChatStateToWaitingString(nextState)))
	}

	var stateCall int
	if nextState < 0 {
		stateCall = nextState
		nextState = ChatStateNone
	}

	objectID, err := primitive.ObjectIDFromHex(id)
	utils.PanicOnError(err)

	_, err = ctx.Store.DB().Collection(collections.CHAT).UpdateOne(ctx.Store.Context, bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	}, bson.M{
		"$set": bson.M{
			"customdata": objectID,
			"state":      nextState,
		},
	})
	utils.PanicOnError(err)

	if stateCall != 0 {
		ChatStateCall(stateCall, c.BotUserName, ctx)
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

	_, err := ctx.Store.DB().Collection(collections.APPS).UpdateOne(ctx.Store.Context, bson.M{"_id": chat.CustomData}, bson.M{
		"$set": bson.M{
			"name":       name,
			"lastreview": time.Time{},
		},
		"$unset": bson.M{
			"lastreviewid": 1,
		},
	})
	utils.PanicOnError(err)

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
	BotUserName string
}

func (c ChangeGroupReceiver) Handle(ctx Context) bool {
	log.Printf("ChangeGroupReceiver")
	var chat Chat
	chatSelector := bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	}
	err := ctx.Store.DB().Collection(collections.CHAT).FindOne(ctx.Store.Context, chatSelector).Decode(&chat)
	utils.PanicOnError(err)

	id := chat.CustomData.(primitive.ObjectID)
	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "Great, check private chat for further instructions.")
	ctx.Resp <- tgbotapi.NewMessage(int64(ctx.UserId()),
		"Use next lint to add me to desired group: https://telegram.me/"+c.BotUserName+"?startgroup="+id.Hex()+"\n Or leave it here: /private_"+id.Hex())

	_, err = ctx.Store.DB().Collection(collections.APPS).UpdateOne(ctx.Store.Context, bson.M{"_id": id}, bson.M{
		"$unset": bson.M{
			"chatid": 1,
		},
	})
	utils.PanicOnError(err)
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
	appObjectID, err := primitive.ObjectIDFromHex(appId)
	utils.PanicOnError(err)

	ctx.BindAppToChatId(appObjectID, int64(ctx.UserId()))

	return true
}

func (ChangeGroupPrivateReceiver) Name() string {
	return "ChangeGroupPrivateReceiver"
}

type ChangeAppStore struct {
	Handler
}

func (ChangeAppStore) Handle(ctx Context) bool {
	if !ctx.EnsureCommand("/changeappstore") {
		return false
	}

	chattable := makeAppChooser(ctx)
	if chattable == nil {
		ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "No apps to change")
		return true
	}

	if !ctx.ChangeChatStateWithNextStateOrAnswerDefault(ChatStateWaitForApp, ChangeAppStoreWaitForCode) {
		return false
	}

	if chattable != nil {
		ctx.Resp <- *chattable
	}

	return true
}

func (ChangeAppStore) Name() string {
	return "ChangeAppStore"
}

type ChangeAppStoreReceiver struct {
	Handler
}

func (ChangeAppStoreReceiver) Handle(ctx Context) bool {
	stateOk, chat := ctx.EnsureChatState(ChangeAppStoreWaitForCode)
	if !stateOk {
		return false
	}

	code := ctx.Update.Message.Text

	_, err := ctx.Store.DB().Collection(collections.APPS).UpdateOne(ctx.Store.Context, bson.M{"_id": chat.CustomData}, bson.M{
		"$set": bson.M{
			"appStoreCountryCode": code,
			"lastreview":          time.Time{},
		},
		"$unset": bson.M{
			"lastreviewid": 1,
		},
	})
	utils.PanicOnError(err)

	ctx.ChangeChatStateWithNextState(ChatStateNone, ChatStateNone)

	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), "App store code changed")
	ctx.AppChanges <- 1

	return true
}

func (ChangeAppStoreReceiver) Name() string {
	return "ChangeAppNameReceiver"
}
