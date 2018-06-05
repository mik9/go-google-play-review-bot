package main

import (
	"golang.org/x/oauth2/google"
	"golang.org/x/net/context"
	"google.golang.org/api/androidpublisher/v3"
	"google-play-review-bot/utils"
	"fmt"
	"github.com/globalsign/mgo"
	"gopkg.in/telegram-bot-api.v4"
	"google-play-review-bot/collections"
	"github.com/globalsign/mgo/bson"
	"google-play-review-bot/handlers"
	"time"
	"bytes"
	"log"
	"github.com/jasonlvhit/gocron"
	"github.com/bugsnag/bugsnag-go"
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

	timeFormatted := r.Time.Format("2006-01-02 15:04")

	header := fmt.Sprintf("%s %s (%d) at %s\n",
		r.AppName,
		r.AppVersion,
		r.AppBuildNumber,
		timeFormatted,
	)
	buffer.WriteString(header)

	if len(r.UserName) > 0 {
		buffer.WriteString(r.UserName)
		buffer.WriteString("\n")
	}

	if len(r.Device) > 0 {
		buffer.WriteString("Device: ")
		buffer.WriteString(r.Device)
	}
	buffer.WriteString(" on Android ")
	buffer.WriteString(sdkIntToString(r.SdkInt))
	buffer.WriteString("\n")

	for i := 1; i <= r.Rating; i++ {
		buffer.WriteString("⭐️")
	}
	for i := r.Rating; i < 5; i++ {
		buffer.WriteString("☆")
	}

	if len(r.Text) > 0 {
		buffer.WriteString("\n")
		buffer.WriteString(r.Text)
	}

	return buffer.String()
}

func sdkIntToString(sdkInt int) string {
	switch sdkInt {
	case 28:
		return "9.0"
	case 27:
		return "8.1"
	case 26:
		return "8.0"
	case 25:
		return "7.1"
	case 24:
		return "7.0"
	case 23:
		return "6.0"
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

func requestReviews(db *mgo.Database, app handlers.Application, respChannel chan tgbotapi.Chattable) {
	defer bugsnag.AutoNotify()
	log.Printf("requestReviews: %s", app.PackageName)

	err := db.C(collections.APPS).FindId(app.ID).One(&app)
	if err != nil {
		utils.LogError(err)
		return
	}

	jsonKey, _ := google.JWTConfigFromJSON(app.KeyFile, androidpublisher.AndroidpublisherScope)

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
		doNext, err, newsetTimeOnPage, nextPageToken = handlePage(reviewService, nextPageToken, app, respChannel)
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

	err = db.C(collections.APPS).UpdateId(app.ID, bson.M{
		"$set": updateFields,
	})
	utils.LogError(err)
}

func handlePage(reviewService *androidpublisher.ReviewsService,
	token string,
	app handlers.Application,
	respChannel chan tgbotapi.Chattable) (bool, error, *time.Time, string) {

	log.Printf("handlePage")

	reviewListCall := reviewService.List(app.PackageName)
	reviewListCall.TranslationLanguage(app.TranslateLanguage)
	if len(token) > 0 && token != "-" {
		reviewListCall.Token(token)
	}

	reviewList, err := reviewListCall.Do()
	if err != nil {
		return false, err, nil, ""
	}
	var newestReview time.Time
	for i, r := range reviewList.Reviews {
		c := r.Comments[0].UserComment
		reviewTime := time.Unix(c.LastModified.Seconds, c.LastModified.Nanos)

		if reviewTime.Before(app.LastReview) || reviewTime.Equal(app.LastReview) {
			log.Printf("Review is older that last time")
			return false, nil, &newestReview, ""
		}

		if reviewTime.After(newestReview) {
			newestReview = reviewTime
		}

		if app.LastReview.IsZero() && i > 0 {
			log.Printf("No reviewTime, allow only one review")
			return false, nil, &newestReview, ""
		}

		handleSingleComment(app, r, c, respChannel)

	}

	var nextToken string
	if reviewList.TokenPagination != nil {
		nextToken = reviewList.TokenPagination.NextPageToken
	} else {
		nextToken = ""
	}
	return true, nil, &newestReview, nextToken
}

func handleSingleComment(app handlers.Application,
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

	fmt.Printf("%+v\n", review)

	respChannel <- tgbotapi.NewMessage(app.ChatId, review.format())
}

func observeApps(db *mgo.Database, respChannel chan tgbotapi.Chattable, appCollectionUpdate chan int) {
	defer bugsnag.AutoNotify()
	for range appCollectionUpdate {
		log.Printf("Got update")
		var apps []handlers.Application
		db.C(collections.APPS).Find(bson.M{
			"keyfile": bson.M{
				"$exists": true,
			},
			"chatid": bson.M{
				"$exists": true,
			},
		}).All(&apps)
		reschedule(db, apps, respChannel)
	}
}

var scheduler = gocron.NewScheduler()

func reschedule(db *mgo.Database, apps []handlers.Application, respChannel chan tgbotapi.Chattable) {
	scheduler.Clear()
	for _, app := range apps {
		scheduler.Every(10).Minutes().Do(requestReviews, db, app, respChannel)
	}
	scheduler.RunAllwithDelay(1000)
	scheduler.Start()
}
