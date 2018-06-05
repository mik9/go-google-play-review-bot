package handlers

import (
	"gopkg.in/telegram-bot-api.v4"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"google-play-review-bot/collections"
	"fmt"
	"google-play-review-bot/utils"
	"strings"
	"io"
	"net/http"
	"log"
)

type Handler interface {
	Handle(ctx Context) bool
	Name() string
}

type Context struct {
	Update     tgbotapi.Update
	Resp       chan tgbotapi.Chattable
	AppChanges chan int
	Db         *mgo.Database
	Bot        *tgbotapi.BotAPI
}

func (ctx Context) EnsureCommand(command string) bool {
	if ctx.Update.Message == nil {
		return false
	}
	return strings.HasPrefix(ctx.Update.Message.Text, command)
}

func (ctx Context) SafeChatId() int64 {
	if ctx.Update.Message != nil {
		return ctx.Update.Message.Chat.ID
	}
	if ctx.Update.CallbackQuery != nil && ctx.Update.CallbackQuery.Message != nil {
		return ctx.Update.CallbackQuery.Message.Chat.ID
	}
	return 0
}


func (ctx Context) ChatId() int64 {
	chatId := ctx.SafeChatId()
	if chatId == 0 {
		panic("don't know where to get chat id")
	}

	return chatId
}

func (ctx Context) UserId() int {
	if ctx.Update.Message != nil {
		return ctx.Update.Message.From.ID
	}
	if ctx.Update.CallbackQuery.Message != nil {
		return ctx.Update.CallbackQuery.From.ID
	}
	panic("don't know where to get user id")
}

func (ctx Context) EnsureChatState(state int) (bool, Chat) {
	chatCollection := ctx.Db.C(collections.CHAT)

	chat := Chat{}
	err := chatCollection.Find(bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	}).One(&chat)

	return err == nil && chat.State == state, chat
}

func (ctx Context) ChangeChatState(newState int) error {
	return ctx.ChangeChatStateWithNextState(newState, ChatStateNone)
}

func (ctx Context) ChangeChatStateWithNextState(newState int, nextState int) error {
	chatCollection := ctx.Db.C(collections.CHAT)

	err := chatCollection.Update(bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	}, bson.M{
		"$set": bson.M{
			"state":      newState,
			"customdata": nextState,
		},
	})

	return err
}

func (ctx Context) ChangeChatStateOrAnswerDefault(newState int) bool {
	return ctx.ChangeChatStateWithNextStateOrAnswerDefault(newState, ChatStateNone)
}

func (ctx Context) ChangeChatStateWithNextStateOrAnswerDefault(newState int, nextState int) bool {
	log.Printf("chatid = %d, userid = %d", ctx.ChatId(), ctx.UserId())
	chatCollection := ctx.Db.C(collections.CHAT)

	info, err := chatCollection.Upsert(bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
		"state":  ChatStateNone,
	}, bson.M{
		"$set": bson.M{
			"state":      newState,
			"customdata": nextState,
		},
	})

	utils.LogError(err)
	utils.LogStruct(info)

	// err = when chat already exists and state not match
	if err == nil && (info.Updated != 0 || info.UpsertedId != nil || info.Matched != 0) {
		return true
	}
	var partialChat struct{ State int }
	chatCollection.Find(bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	}).Select(bson.M{"state": 1}).One(&partialChat)

	if partialChat.State == newState {
		return true
	}

	utils.LogStruct(partialChat)

	responseMessage := fmt.Sprintf("I'm waiting for: %s\n", ChatStateToWaitingString(partialChat.State))
	ctx.Resp <- tgbotapi.NewMessage(ctx.ChatId(), responseMessage)
	return false
}

func (ctx Context) downloadFile(fileId string) (io.ReadCloser, error) {
	url, err := ctx.Bot.GetFileDirectURL(fileId)

	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (ctx Context) SetKeyFile(buf []byte) {
	ctx.Db.C(collections.APPS).Update(bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
		"keyfile": bson.M{
			"$exists": false,
		},
	}, bson.M{
		"$set": bson.M{
			"keyfile": buf,
		},
	})

	ctx.AppChanges <- 0
}

func (ctx Context) MigrateChatId(oldId int64, newId int64) {
	err := ctx.Db.C(collections.CHAT).Update(bson.M{
		"chatid": oldId,
	}, bson.M{
		"$set": bson.M{
			"chatid": newId,
		},
	})
	if err != nil {
		utils.LogError(err)
	}

	err = ctx.Db.C(collections.APPS).Update(bson.M{
		"chatid": oldId,
	}, bson.M{
		"$set": bson.M{
			"chatid": newId,
		},
	})
	if err != nil {
		utils.LogError(err)
	}
}

func (ctx Context) BindAppToChatId(appId bson.ObjectId, chatId int64)  {
	err := ctx.Db.C(collections.APPS).Update(bson.M{
		"_id": appId,
		"chatid": bson.M{
			"$exists": false,
		},
	}, bson.M{
		"$set": bson.M{
			"chatid": chatId,
		},
		"$unset": bson.M{
			"lastreview": 1,
		},
	})
	if err != nil {
		panic(err)
	}

	ctx.AppChanges <- 1
}