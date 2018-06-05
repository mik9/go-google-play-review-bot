package main

import (
	"github.com/globalsign/mgo"
	"fmt"
	"os"
	"gopkg.in/telegram-bot-api.v4"
	"log"
	"google-play-review-bot/handlers"
	"google-play-review-bot/collections"
	"runtime/debug"
	"net/http"
	"google-play-review-bot/utils"
	"strings"
	"github.com/globalsign/mgo/bson"
	"github.com/bugsnag/bugsnag-go"
	"reflect"
)

var BotToken = os.Getenv("TELEGRAM_TOKEN")

var Handlers = []handlers.Handler{
	handlers.EditMessageConsumer{}, // we don't handle edit message events
	handlers.Reset{},
	handlers.MigrateHandler{},
	handlers.StartHandler{},

	handlers.NewAppHandler{},
	handlers.PackageNameReceiver{},
	handlers.KeyReceiver{},
	handlers.AppList{},

	handlers.ChangeLanguage{},
	handlers.ChangeLanguageReceiver{},
	handlers.ChangeAppName{},
	handlers.ChangeAppNameReceiver{},
	handlers.ChangeGroup{},
	handlers.ChangeGroupPrivateReceiver{},
	handlers.ChooseAppReceiver{},

	//handlers.DefaultHandler{},
}

func runBot(db *mgo.Database, respChannel chan tgbotapi.Chattable, appChanges chan int) {
	bot, err := tgbotapi.NewBotAPI(BotToken)
	if err != nil {
		panic(err)
	}
	//bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	var updateChannel tgbotapi.UpdatesChannel
	tlsCert, useWebhook := os.LookupEnv("TLS_CERT")
	if useWebhook {
		log.Printf("Using webhook")

		tlsKey := os.Getenv("TLS_KEY")
		webHookUrl := os.Getenv("WEBHOOK_URL")

		bot.SetWebhook(tgbotapi.NewWebhook(webHookUrl + "/" + bot.Token))
		updateChannel = bot.ListenForWebhook("/" + bot.Token)
		go http.ListenAndServeTLS(":8443", tlsCert, tlsKey, nil)
	} else {
		log.Printf("Using getUpdate")
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 600
		updateChannel, _ = bot.GetUpdatesChan(u)
	}

	for {
		select {
		case update := <-updateChannel:
			//go logMessage(db, update)
			go runHandlers(update, respChannel, db, bot, appChanges)
		case resp := <-respChannel:
			_, e := bot.Send(resp)
			if te, ok := e.(tgbotapi.Error); ok && strings.Contains(te.Message, "Forbidden") {
				dropChat(db, resp.(tgbotapi.MessageConfig).ChatID)
			} else {
				utils.LogError(e)
			}
		}
	}
}

func dropChat(db *mgo.Database, id int64) {
	err := db.C(collections.CHAT).Remove(bson.M{
		"chatid": id,
	})
	utils.LogError(err)
	err = db.C(collections.APPS).Remove(bson.M{
		"chatid": id,
	})
	utils.LogError(err)
}

func logMessage(db *mgo.Database, update tgbotapi.Update) {
	err := db.C(collections.MESSAGE_LOG).Insert(update)
	utils.LogError(err)
}

func runHandlers(update tgbotapi.Update, respChannel chan tgbotapi.Chattable, db *mgo.Database, bot *tgbotapi.BotAPI, appChanges chan int) {
	//noinspection GoStructInitializationWithoutFieldNames
	c := handlers.Context{update, respChannel, appChanges, db, bot}
	defer func() {
		if r := recover(); r != nil {
			bugsnag.Notify(utils.MakeError(r))
			logMessage(db, update)
			log.Printf("Panic in handler: %s %s\n%s", reflect.TypeOf(r), r, debug.Stack())
			chatId := c.SafeChatId()
			if chatId != 0 {
				respChannel <- tgbotapi.NewMessage(c.ChatId(), "Error occurred, try again...")
			}
		}
	}()
	for _, h := range Handlers {
		if h.Handle(c) {
			log.Printf("Handled by %s", h.Name())
			break
		}
	}
}

func initDb(db *mgo.Database) {
	chatIndex := mgo.Index{
		Key:        []string{"chatid", "userid"},
		Unique:     true,
		DropDups:   true,
		Background: true,
	}
	db.C(collections.CHAT).EnsureIndex(chatIndex)

	appIndex := mgo.Index{
		Key:        []string{"packagename", "chatid", "userid"},
		Unique:     true,
		DropDups:   true,
		Background: true,
	}

	db.C(collections.APPS).EnsureIndex(appIndex)
}

func main() {
	bugsnagStage, ok := os.LookupEnv("BUGSNAG_STAGE")
	if !ok {
		bugsnagStage = "prod"
	}
	bugsnag.Configure(bugsnag.Configuration{
		APIKey: os.Getenv("BUGSNAG_API_KEY"),
		// The import paths for the Go packages
		// containing your source files
		ProjectPackages: []string{"main*", "google-play-review-bot*"},
		ReleaseStage:    bugsnagStage,
	})

	mongoHost := os.Getenv("MONGO_HOST")
	session, err := mgo.Dial(mongoHost)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Connection succesfull!\n")

	db := session.DB("review-bot")
	fmt.Printf("Got db %s\n", db.Name)

	//db.DropDatabase()
	initDb(db)

	respChannel := make(chan tgbotapi.Chattable, 5)
	appChanges := make(chan int, 5)
	go observeApps(db, respChannel, appChanges)
	appChanges <- 0

	runBot(db, respChannel, appChanges)
}
