package main

import (
	"bytes"
	"fmt"
	"google-play-review-bot/collections"
	"google-play-review-bot/datastore"
	"google-play-review-bot/handlers"
	"google-play-review-bot/scheduler"
	"google-play-review-bot/utils"
	"log"
	"strings"
	"time"

	"github.com/bugsnag/bugsnag-go"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/androidpublisher/v3"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

type userReview struct {
	UserName       string
	Device         string
	SdkInt         int
	Text           string
	Time           time.Time
	Rating         int
	AppVersion     string
	AppBuildNumber int64
	AppName        string
}

func (r userReview) format() string {
	var buffer bytes.Buffer

	var header string
	if r.Time.IsZero() {
		header = fmt.Sprintf("%s %s\n",
			r.AppName,
			r.AppVersion,
		)
	} else {
		timeFormatted := r.Time.Format("2006-01-02 15:04")

		header = fmt.Sprintf("%s %s (%d) at %s\n",
			r.AppName,
			r.AppVersion,
			r.AppBuildNumber,
			timeFormatted,
		)
	}

	buffer.WriteString(header)

	if len(r.UserName) > 0 {
		buffer.WriteString(r.UserName)
		buffer.WriteString("\n")
	}

	if r.Device != "" {
		buffer.WriteString("Device: ")
		buffer.WriteString(r.Device)
	}
	if r.SdkInt > 0 {
		buffer.WriteString(" on Android ")
		buffer.WriteString(sdkIntToString(r.SdkInt))
	}
	if r.Device != "" || r.SdkInt > 0 {
		buffer.WriteString("\n")
	}

	hearticon := "💔"
	if r.Rating > 3 {
		hearticon = "❤️"
	}

	for i := 1; i <= r.Rating; i++ {
		buffer.WriteString(hearticon)
	}

	// for i := r.Rating; i < 5; i++ {
	// 	buffer.WriteString("☆")
	// }

	if len(r.Text) > 0 {
		buffer.WriteString("\n")
		buffer.WriteString(strings.TrimSpace(r.Text))
	}

	return buffer.String()
}

func sdkIntToString(sdkInt int) string {
	switch sdkInt {
	case 33:
		return "13"
	case 32:
		return "12L"
	case 31:
		return "12"
	case 30:
		return "11"
	case 29:
		return "10"
	case 28:
		return "9"
	case 27:
		return "8.1"
	case 26:
		return "8.0"
	case 25:
		return "7.1"
	case 24:
		return "7.0"
	case 23:
		return "6"
	case 22:
		return "5.1"
	case 21:
		return "5.0"
	case 20:
		return "4.4W o_O"
	case 19:
		return "4.4"
	case 18:
		return "4.3"
	case 17:
		return "4.2"
	case 16:
		return "4.1"
	case 15:
		return "4.0.4"
	case 14:
		return "4.0"
	default:
		return fmt.Sprintf("Unknown (%d)", sdkInt)
	}
}

type AndroidAppObserver struct {
	scheduler *scheduler.Scheduler
}

var _ AppObserver = (*AndroidAppObserver)(nil)

func (a AndroidAppObserver) Observe(respChannel chan tgbotapi.Chattable, appCollectionUpdate chan int) {
	a.scheduler = scheduler.NewScheduler()
	observe("android", respChannel, appCollectionUpdate, a.rescheduleAndroid)
}

func (a AndroidAppObserver) requestReviews(app handlers.Application, respChannel chan tgbotapi.Chattable) {
	defer bugsnag.AutoNotify()
	log.Printf("[%s] requestReviews", app.PackageName)

	datastore.Use(func(store *datastore.Datastore) {
		err := store.DB().Collection(collections.APPS).FindOne(store.Context, bson.M{"_id": app.ID}).Decode(&app)
		utils.PanicOnError(err)
	})

	jsonKey, err := google.JWTConfigFromJSON(app.KeyFile, androidpublisher.AndroidpublisherScope)
	utils.PanicOnError(err)

	client := jsonKey.Client(context.Background())
	service, err := androidpublisher.New(client)
	if err != nil {
		utils.LogError(err)
		return
	}
	reviewService := service.Reviews

	pageLimit := 2
	doNext := true
	nextPageToken := "-"
	var newestReviewTime time.Time
	for i := 0; doNext && len(nextPageToken) > 0 && i < pageLimit; i++ {
		var newsetTimeOnPage *time.Time
		doNext, newsetTimeOnPage, nextPageToken, err = a.handlePage(reviewService, nextPageToken, app, respChannel)
		if err != nil {
			utils.LogError(err)
			return
		}
		if newsetTimeOnPage.After(newestReviewTime) {
			newestReviewTime = *newsetTimeOnPage
		}
	}

	var updateFields = bson.M{
		"lastreviewsqueried": time.Now(),
	}
	if !newestReviewTime.IsZero() {
		updateFields["lastreview"] = newestReviewTime
	}

	datastore.Use(func(store *datastore.Datastore) {
		u, err := store.DB().Collection(collections.APPS).UpdateOne(store.Context, bson.M{"_id": app.ID}, bson.M{
			"$set": updateFields,
		})
		if err == nil && u.MatchedCount == 0 {
			utils.LogError(fmt.Errorf("Not updated app"))
		}
		utils.LogError(err)
	})
}

func (a AndroidAppObserver) handlePage(reviewService *androidpublisher.ReviewsService,
	token string,
	app handlers.Application,
	respChannel chan tgbotapi.Chattable) (bool, *time.Time, string, error) {

	log.Printf("handlePage [%s]", app.PackageName)

	reviewListCall := reviewService.List(app.PackageName)
	reviewListCall.TranslationLanguage(app.TranslateLanguage)
	if len(token) > 0 && token != "-" {
		reviewListCall.Token(token)
	}

	reviewList, err := reviewListCall.Do()
	if err != nil {
		return false, nil, "", err
	}
	if Debug {
		utils.LogStruct(reviewList)
	}
	log.Printf("handlePage [%s] review count: %d", app.PackageName, len(reviewList.Reviews))
	var newestReview time.Time
	for i, r := range reviewList.Reviews {
		c := r.Comments[0].UserComment
		reviewTime := time.Unix(c.LastModified.Seconds, c.LastModified.Nanos)

		if reviewTime.Before(app.LastReview) || reviewTime.Equal(app.LastReview) {
			log.Printf("handlePage [%s]: Review is older that last time", app.PackageName)
			return false, &newestReview, "", nil
		}

		if reviewTime.After(newestReview) {
			newestReview = reviewTime
		}

		if app.LastReview.IsZero() && i > 0 {
			log.Printf("handlePage [%s] No reviewTime, allow only one review", app.PackageName)
			return false, &newestReview, "", nil
		}

		a.handleSingleComment(app, r, c, respChannel)
	}

	var nextToken string
	if reviewList.TokenPagination != nil {
		nextToken = reviewList.TokenPagination.NextPageToken
	} else {
		nextToken = ""
	}
	return true, &newestReview, nextToken, nil
}

func (a AndroidAppObserver) handleSingleComment(app handlers.Application,
	r *androidpublisher.Review,
	c *androidpublisher.UserComment,
	respChannel chan tgbotapi.Chattable) {
	lastModified := time.Unix(c.LastModified.Seconds, c.LastModified.Nanos)

	var deviceName string
	if c.DeviceMetadata != nil && len(c.DeviceMetadata.ProductName) != 0 {
		deviceName = c.DeviceMetadata.ProductName
	} else {
		deviceName = c.Device
	}
	review := userReview{
		UserName:       r.AuthorName,
		Device:         deviceName,
		SdkInt:         int(c.AndroidOsVersion),
		Text:           c.Text,
		Time:           lastModified,
		Rating:         int(c.StarRating),
		AppVersion:     c.AppVersionName,
		AppBuildNumber: c.AppVersionCode,
		AppName:        app.GetName(),
	}

	log.Printf("[Android] Sending message to %d", app.ChatId)
	respChannel <- tgbotapi.NewMessage(app.ChatId, review.format())
}

func (a AndroidAppObserver) rescheduleAndroid(apps []handlers.Application, respChannel chan tgbotapi.Chattable) {
	a.scheduler.Clear()
	log.Printf("[Android] Scheduling %d apps", len(apps))
	for _, app := range apps {
		_app := app
		a.scheduler.Schedule(func() {
			a.requestReviews(_app, respChannel)
		}, 10*time.Minute)
	}
}
