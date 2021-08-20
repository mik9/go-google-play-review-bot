package datastore

import (
	"context"
	"google-play-review-bot/collections"
	"google-play-review-bot/utils"
	"log"
	"os"
	"time"

	"github.com/bugsnag/bugsnag-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const db = "review-bot"

var client *mongo.Client

type Datastore struct {
	client  *mongo.Client
	Context context.Context
}

func init() {
	var err error
	mongoHost := os.Getenv("MONGO_HOST")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("Connecting to %s", mongoHost)
	client, err = mongo.Connect(ctx, options.Client().ApplyURI(mongoHost))
	if err != nil {
		panic(err)
	}

	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		panic(err)
	}
	log.Printf("Connected")

	go func() {
		bugsnag.AutoNotify()

		store, cancel := Get()
		defer cancel()

		chatIndex := mongo.IndexModel{
			Keys:    bson.D{{"chatid", 1}, {"userid", 1}},
			Options: options.Index().SetUnique(true).SetBackground(true),
		}
		_, err := DB().Collection(collections.CHAT).Indexes().CreateOne(store.Context, chatIndex)
		utils.PanicOnError(err)

		appIndex := mongo.IndexModel{
			Keys:    bson.D{{"packagename", 1}, {"chatid", 1}, {"userid", 1}},
			Options: options.Index().SetUnique(true).SetBackground(true),
		}

		_, err = DB().Collection(collections.APPS).Indexes().CreateOne(store.Context, appIndex)
		utils.PanicOnError(err)
	}()

	updateAppType()
}

func updateAppType() {
	store, cancel := Get()
	defer cancel()

	store.DB().Collection(collections.APPS).UpdateMany(store.Context, bson.M{
		"os": bson.M{"$exists": false},
	}, bson.M{
		"$set": bson.M{
			"os": "android",
		},
	})
}

func CreateDefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func Get() (*Datastore, context.CancelFunc) {
	ctx, cancel := CreateDefaultContext()

	return &Datastore{
		client:  client,
		Context: ctx,
	}, cancel
}

func Use(fn func(*Datastore)) {
	store, cancel := Get()
	defer cancel()

	fn(store)
}

func DB() *mongo.Database {
	return client.Database(db)
}

func (store *Datastore) DB() *mongo.Database {
	return store.client.Database(db)
}
