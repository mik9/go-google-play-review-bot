package handlers

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Application struct {
	ChatId              int64              `bson:",omitempty"`
	UserId              int                `bson:",omitempty"`
	ID                  primitive.ObjectID `bson:"_id,omitempty"`
	PackageName         string
	AppStoreCountryCode string     `bson:"appStoreCountryCode,omitempty"`
	Name                string     `bson:",omitempty"`
	KeyFile             []byte     `bson:",omitempty"`
	LastReviewsQueried  *time.Time `bson:",omitempty"`
	LastReview          time.Time  `bson:",omitempty"`
	LastReviewId        string     `bson:",omitempty"`
	TranslateLanguage   string
}

func (a Application) GetName() string {
	if len(a.Name) == 0 {
		return a.PackageName
	}

	return a.Name
}

type Chat struct {
	ChatId     int64              `bson:",omitempty"`
	UserId     int                `bson:",omitempty"`
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	State      int                `bson:",omitempty"`
	CustomData interface{}        `bson:",omitempty"`
}

const (
	ChatStateNone               = 0
	ChatStateWaitForPackageName = 1
	ChatStateWaitForKey         = 2
	ChatStateWaitForApp         = 3
	ChatStateWaitForLanguage    = 4
	ChatStateWaitForAppName     = 5
	ChatStateWaitForOS          = 6
	ChatStateWaitForIosAppID    = 7
	ChangeAppStoreWaitForCode   = 8
)

func ChatStateToWaitingString(state int) string {
	switch state {
	case ChatStateNone:
		return "nothing"
	case ChatStateWaitForPackageName:
		return "package name"
	case ChatStateWaitForKey:
		return "json key"
	case ChatStateWaitForApp:
		return "choose app"
	case ChatStateWaitForLanguage:
		return "language code"
	case ChatStateWaitForAppName:
		return "application name"
	case ChatStateWaitForOS:
		return "os selection"
	case ChatStateWaitForIosAppID:
		return "ios app id"
	case ChangeAppStoreWaitForCode:
		return "AppStore code"
	}

	panic(UnknownStateError{state: state})
}

type UnknownStateError struct {
	error
	state int
}

func (e UnknownStateError) Error() string {
	return fmt.Sprintf("Cannot convert state to string: %d", e.state)
}

const (
	ChatStateCallChangeGroupReceiver = -1
)

func ChatStateCall(state int, botUserName string, ctx Context) {
	switch state {
	case ChatStateCallChangeGroupReceiver:
		ChangeGroupReceiver{
			BotUserName: botUserName,
		}.Handle(ctx)
	}
}
