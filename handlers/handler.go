package handlers

import (
	"fmt"
	"google-play-review-bot/collections"
	"google-play-review-bot/datastore"
	"google-play-review-bot/utils"
	"io"
	"log"
	"net/http"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

type Handler interface {
	Handle(ctx Context) bool
	Name() string
}

type Context struct {
	Update     tgbotapi.Update
	Resp       chan tgbotapi.Chattable
	AppChanges chan int
	Store      *datastore.Datastore
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

func (ctx Context) EnsureChatState(state int) (bool, *Chat) {
	chatCollection := ctx.Store.DB().Collection(collections.CHAT)

	chat := Chat{}
	r := chatCollection.FindOne(ctx.Store.Context, bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	})

	if err := r.Err(); err != nil {
		return false, nil
	}

	err := r.Decode(&chat)

	return err == nil && chat.State == state, &chat
}

func (ctx Context) ChangeChatState(newState int) error {
	return ctx.ChangeChatStateWithNextState(newState, ChatStateNone)
}

func (ctx Context) ChangeChatStateWithNextState(newState int, nextState int) error {
	_, err := ctx.Store.DB().Collection(collections.CHAT).UpdateOne(ctx.Store.Context, bson.M{
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

	info, err := ctx.Store.DB().Collection(collections.CHAT).UpdateOne(ctx.Store.Context, bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
		"state":  ChatStateNone,
	}, bson.M{
		"$set": bson.M{
			"state":      newState,
			"customdata": nextState,
		},
	}, options.Update().SetUpsert(true))

	utils.LogError(err)
	utils.LogStruct(info)

	// err = when chat already exists and state not match
	if err == nil && (info.ModifiedCount != 0 || info.UpsertedID != nil || info.MatchedCount != 0) {
		return true
	}

	var partialChat struct{ State int }
	ctx.Store.DB().Collection(collections.CHAT).FindOne(ctx.Store.Context, bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
	}, options.FindOne().SetProjection(bson.M{"state": 1})).Decode(&partialChat)

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
	_, err := ctx.Store.DB().Collection(collections.APPS).UpdateOne(ctx.Store.Context, bson.M{
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

	utils.PanicOnError(err)

	ctx.AppChanges <- 0
}

func (ctx Context) MigrateChatId(oldId int64, newId int64) {
	_, err := ctx.Store.DB().Collection(collections.CHAT).UpdateMany(ctx.Store.Context, bson.M{
		"chatid": oldId,
	}, bson.M{
		"$set": bson.M{
			"chatid": newId,
		},
	})
	if err != nil {
		utils.LogError(err)
	}

	_, err = ctx.Store.DB().Collection(collections.APPS).UpdateMany(ctx.Store.Context, bson.M{
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

func (ctx Context) BindAppToChatId(appId primitive.ObjectID, chatId int64) {
	_, err := ctx.Store.DB().Collection(collections.APPS).UpdateOne(ctx.Store.Context, bson.M{
		"_id": appId,
		"chatid": bson.M{
			"$exists": false,
		},
	}, bson.M{
		"$set": bson.M{
			"chatid": chatId,
		},
		"$unset": bson.M{
			"lastreview":   1,
			"lastreviewid": 1,
		},
	})
	utils.PanicOnError(err)
	log.Printf("[BindAppToChatId] appId: %v, chatId: %v", appId, chatId)

	ctx.AppChanges <- 1
}

func (ctx Context) SaveOS(os string) primitive.ObjectID {
	res, err := ctx.Store.DB().Collection(collections.APPS).InsertOne(ctx.Store.Context, bson.M{
		"chatid":              ctx.ChatId(),
		"userid":              ctx.UserId(),
		"os":                  os,
		"translatelanguage":   "en",
		"appStoreCountryCode": "us",
	})
	utils.PanicOnError(err)

	return res.InsertedID.(primitive.ObjectID)
}

func (ctx Context) SavePackageName(packageName string) error {
	_, err := ctx.Store.DB().Collection(collections.APPS).UpdateOne(ctx.Store.Context, bson.M{
		"chatid": ctx.ChatId(),
		"userid": ctx.UserId(),
		"packagename": bson.M{
			"$exists": false,
		},
	}, bson.M{
		"$set": bson.M{
			"packagename": packageName,
		},
	})
	return err
}
