package main

import (
	"encoding/json"
	"fmt"
	"google-play-review-bot/collections"
	"google-play-review-bot/datastore"
	"google-play-review-bot/handlers"
	"google-play-review-bot/scheduler"
	"google-play-review-bot/utils"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/bugsnag/bugsnag-go"
	"go.mongodb.org/mongo-driver/bson"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

type rss struct {
	Feed rssFeed `json:"feed"`
}

type rssFeed struct {
	Entry []entry `json:"entry"`
}

type entry struct {
	ID      wrappedText  `json:"id"`
	Author  reviewAuthor `json:"author"`
	Version wrappedText  `json:"im:version"`
	Rating  wrappedText  `json:"im:rating"`
	Title   wrappedText  `json:"title"`
	Content wrappedText  `json:"content"`
}

type reviewAuthor struct {
	Name wrappedText `json:"name"`
}

type wrappedText string

var _ json.Unmarshaler = (*wrappedText)(nil)

func (t *wrappedText) UnmarshalJSON(data []byte) error {
	titleStruct := struct {
		Label string `json:"label"`
	}{}

	err := json.Unmarshal(data, &titleStruct)
	if err != nil {
		return err
	}

	*t = wrappedText(titleStruct.Label)
	return nil
}

type IosAppObserver struct {
	scheduler *scheduler.Scheduler
}

var _ AppObserver = (*IosAppObserver)(nil)

func (i IosAppObserver) requestReviews(app handlers.Application, respChannel chan tgbotapi.Chattable) {
	log.Printf("[iOS, %s, %s, %s] requestReviews", app.Name, app.ID.Hex(), app.PackageName)
	defer bugsnag.AutoNotify(bugsnag.MetaData{"app": {"id": app.ID.Hex(), "packageName": app.PackageName}})

	datastore.Use(func(store *datastore.Datastore) {
		err := store.DB().Collection(collections.APPS).FindOne(store.Context, bson.M{"_id": app.ID}).Decode(&app)
		utils.PanicOnError(err)
	})

	url := fmt.Sprintf("https://itunes.apple.com/%s/rss/customerreviews/id=%s/sortBy=mostRecent/json", app.AppStoreCountryCode, app.PackageName)
	if Debug {
		log.Printf("[iOS] %s", url)
	}

	resp, err := http.Get(url)
	utils.LogError(err)

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		utils.PanicOnError(err)

		utils.LogError(fmt.Errorf("Got error: %s\n%s", resp.Status, string(body)))
		return
	}

	rss := rss{}
	err = json.NewDecoder(resp.Body).Decode(&rss)
	utils.LogError(err)

	if Debug {
		utils.LogStruct(rss)
	}

	processedReviewID := ""
	if app.LastReviewId != "" {
		found := false
		for _, rssEntry := range rss.Feed.Entry {
			if string(rssEntry.ID) == app.LastReviewId {
				found = true
				break
			}
		}
		if !found && len(rss.Feed.Entry) > 0 {
			app.LastReviewId = string(rss.Feed.Entry[0].ID)
			processedReviewID = app.LastReviewId
		}
	}

	for _, rssEntry := range rss.Feed.Entry {
		if app.LastReviewId == string(rssEntry.ID) {
			break
		}

		rating, err := strconv.ParseInt(string(rssEntry.Rating), 10, 16)
		if err != nil {
			utils.LogError(err)
			continue
		}

		review := userReview{
			UserName:   string(rssEntry.Author.Name),
			Text:       fmt.Sprintf("%s\n%s", rssEntry.Title, rssEntry.Content),
			Rating:     int(rating),
			AppVersion: string(rssEntry.Version),
			AppName:    app.GetName(),
		}

		respChannel <- tgbotapi.NewMessage(app.ChatId, review.format())

		if processedReviewID == "" {
			processedReviewID = string(rssEntry.ID)
		}

		if app.LastReviewId == "" {
			break
		}
	}

	if processedReviewID != "" {
		datastore.Use(func(store *datastore.Datastore) {
			_, err := store.DB().Collection(collections.APPS).UpdateOne(store.Context, bson.M{"_id": app.ID},
				bson.M{
					"$set": bson.M{
						"lastreviewid": processedReviewID,
					},
				})
			utils.PanicOnError(err)
		})
	}
}

func (i IosAppObserver) Observe(respChannel chan tgbotapi.Chattable, appCollectionUpdate chan int) {
	i.scheduler = scheduler.NewScheduler()
	observe("ios", respChannel, appCollectionUpdate, i.rescheduleIos)
}

func (i IosAppObserver) rescheduleIos(apps []handlers.Application, respChannel chan tgbotapi.Chattable) {
	i.scheduler.Clear()
	log.Printf("[iOS] Scheduling %d apps", len(apps))
	for _, app := range apps {
		_app := app
		i.scheduler.Schedule(func() {
			i.requestReviews(_app, respChannel)
		}, 10*time.Minute)
	}
}
