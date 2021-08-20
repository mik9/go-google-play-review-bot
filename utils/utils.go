package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"

	"github.com/bugsnag/bugsnag-go"
)

func LogError(err error) {
	if err != nil {
		bugsnag.Notify(err)
		log.Printf("ERROR: %s", err.Error())
	}
}

func MakeError(v interface{}) error {
	switch f := v.(type) {
	case error:
		return f
	case string:
		return fmt.Errorf(f)
	}

	return fmt.Errorf("unknown error: %s, %s", reflect.TypeOf(v), v)
}

func LogStruct(s interface{}) {
	if s != nil {
		b, _ := json.Marshal(s)
		log.Println(string(b))
	}
}

func PanicOnError(err error) {
	if err != nil {
		bugsnag.Notify(err)
		log.Printf("ERROR: %s", err.Error())
		panic(err)
	}
}
