package main

import (
	"google-play-review-bot/collections"
	"google-play-review-bot/datastore"
	"google-play-review-bot/handlers"
	"google-play-review-bot/utils"
	"log"
	"net/http"
	"os"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/bugsnag/bugsnag-go"
	"go.mongodb.org/mongo-driver/bson"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

var BotToken = os.Getenv("TELEGRAM_TOKEN")
var Debug = os.Getenv("DEBUG") != ""

var Handlers []handlers.Handler

func initHandlers(botUserName string) {
	Handlers = []handlers.Handler{
		handlers.EditMessageConsumer{}, // we don't handle edit message events
		handlers.Reset{},
		handlers.MigrateHandler{},
		handlers.StartHandler{},

		handlers.NewAppHandler{},
		handlers.IosAndroidHandler{},
		handlers.PackageNameReceiver{},
		handlers.KeyReceiver{},
		handlers.AppList{},

		handlers.ChangeLanguage{},
		handlers.ChangeLanguageReceiver{},
		handlers.ChangeAppName{},
		handlers.ChangeAppNameReceiver{},
		handlers.ChangeGroup{},
		handlers.ChangeGroupPrivateReceiver{},
		handlers.ChooseAppReceiver{
			BotUserName: botUserName,
		},
		handlers.ChangeAppStore{},
		handlers.ChangeAppStoreReceiver{},

		//handlers.DefaultHandler{},
	}
}

func runBot(respChannel chan tgbotapi.Chattable, appChanges chan int) {
	bot, err := tgbotapi.NewBotAPI(BotToken)
	utils.PanicOnError(err)
	// bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	var updateChannel tgbotapi.UpdatesChannel
	webHookUrl, useWebhook := os.LookupEnv("WEBHOOK_URL")
	if useWebhook {
		log.Printf("Using webhook")

		tlsCert := os.Getenv("TLS_CERT")
		tlsKey := os.Getenv("TLS_KEY")

		bot.SetWebhook(tgbotapi.NewWebhook(webHookUrl + "/" + bot.Token))
		updateChannel = bot.ListenForWebhook("/" + bot.Token)
		if tlsCert != "" {
			log.Printf("Using https")
			go http.ListenAndServeTLS(":8443", tlsCert, tlsKey, nil)
		} else {
			log.Printf("Using http")
			go http.ListenAndServe(":8443", nil)
		}
	} else {
		log.Printf("Using getUpdate")
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 600
		updateChannel, err = bot.GetUpdatesChan(u)
		utils.PanicOnError(err)
	}

	botInfo, err := bot.GetMe()
	utils.PanicOnError(err)

	log.Printf("Bot name: %s", botInfo.UserName)

	initHandlers(botInfo.UserName)

	for {
		select {
		case update := <-updateChannel:
			if Debug {
				go logMessage(update)
			}
			go runHandlers(update, respChannel, bot, appChanges)
		case resp := <-respChannel:
			_, e := bot.Send(resp)
			if te, ok := e.(tgbotapi.Error); ok && strings.Contains(te.Message, "Forbidden") {
				dropChat(resp.(tgbotapi.MessageConfig).ChatID)
			} else {
				utils.LogError(e)
			}
		}
	}
}

func dropChat(id int64) {
	store, cancel := datastore.Get()
	defer cancel()

	_, err := store.DB().Collection(collections.CHAT).DeleteOne(store.Context, bson.M{
		"chatid": id,
	})
	utils.LogError(err)

	_, err = store.DB().Collection(collections.APPS).DeleteOne(store.Context, bson.M{
		"chatid": id,
	})
	utils.LogError(err)
}

func logMessage(update tgbotapi.Update) {
	store, cancel := datastore.Get()
	defer cancel()

	_, err := store.DB().Collection(collections.MESSAGE_LOG).InsertOne(store.Context, update)
	utils.LogError(err)
}

func runHandlers(update tgbotapi.Update, respChannel chan tgbotapi.Chattable, bot *tgbotapi.BotAPI, appChanges chan int) {
	store, cancel := datastore.Get()
	defer cancel()

	//noinspection GoStructInitializationWithoutFieldNames
	c := handlers.Context{update, respChannel, appChanges, store, bot}
	defer func() {
		if r := recover(); r != nil {
			bugsnag.Notify(utils.MakeError(r))
			logMessage(update)
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
			return
		}
	}
}

func observe(
	os string,
	respChannel chan tgbotapi.Chattable,
	appCollectionUpdate chan int,
	reschedule func([]handlers.Application, chan tgbotapi.Chattable)) {

	defer bugsnag.AutoNotify()
	for range appCollectionUpdate {
		log.Printf("Got app update")

		store, cancel := datastore.Get()
		defer cancel()

		findQuery := bson.M{
			"os": os,
			"chatid": bson.M{
				"$exists": true,
			},
		}
		if os == "android" {
			findQuery["keyfile"] = bson.M{
				"$exists": true,
			}
		}
		if os == "ios" {
			findQuery["appStoreCountryCode"] = bson.M{
				"$exists": true,
			}
		}
		c, err := store.DB().Collection(collections.APPS).Find(store.Context, findQuery)

		utils.PanicOnError(err)

		var apps []handlers.Application
		err = c.All(store.Context, &apps)
		utils.PanicOnError(err)

		reschedule(apps, respChannel)
	}
}

type AppObserver interface {
	Observe(respChannel chan tgbotapi.Chattable, appCollectionUpdate chan int)
}

var observers = []AppObserver{
	AndroidAppObserver{},
	IosAppObserver{},
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

	respChannel := make(chan tgbotapi.Chattable, 5)
	appChanges := make(chan int, 5)

	observerChannels := []chan int{}

	for _, observer := range observers {
		appChanges := make(chan int, 5)
		observerChannels = append(observerChannels, appChanges)
		go observer.Observe(respChannel, appChanges)
	}

	go func() {
		for n := range appChanges {
			for _, c := range observerChannels {
				c <- n
			}
		}
	}()

	appChanges <- 0

	runBot(respChannel, appChanges)
}
