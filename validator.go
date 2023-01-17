package work

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
)

// use a single instance of Validate, it caches struct info
var validate *validator.Validate
var translations map[string]map[string]string

const lang = "pt" // TODO - config

type Error struct {
	Field   string
	Message string
}

type validateInterface interface {
	Validate() []Error
}

func NewValidator() {
	validate = validator.New()

	err := registerTranslations()
	if err != nil {
		panic(err)
	}
}

func registerTranslations() error {
	configFile, _ := os.Open("./config.json") // TODO - constant or something
	defer configFile.Close()

	jsonFile, err := ioutil.ReadAll(configFile)
	if err != nil {
		return err
	}

	json.Unmarshal([]byte(jsonFile), &translations)

	return nil
}

func fieldTranslation(fe validator.FieldError) string {
	var match string

	if len(fe.Namespace()) > 0 {
		match := translations[lang][fe.Namespace()]
		if len(match) > 0 {
			return match
		}
	}

	match = translations[lang][fe.Field()]
	if len(match) > 0 {
		return match
	}

	return fe.Field() // default field name
}

func truncatedSprintf(str string, args ...interface{}) (string, error) {
	n := strings.Count(str, "%s")
	if n > len(args) {
		return "", errors.New("Unexpected string:" + str)
	}
	return fmt.Sprintf(str, args[:n]...), nil
}

func errorTranslation(fe validator.FieldError) string {
	match := translations[lang][fe.Tag()]
	if len(match) > 0 {
		result, err := truncatedSprintf(match, fieldTranslation(fe), fe.Param())
		if err != nil {
			log.Fatal(err)
		}

		return result
	}

	return fe.Error()
}

func validationTranslate(ve validator.ValidationErrors) []Error {
	out := make([]Error, len(ve))

	for i, fe := range ve {
		out[i] = Error{fe.Field(), errorTranslation(fe)}
	}

	return out
}

func Validate[K validateInterface](instance K) []Error {
	err := validate.Struct(instance)
	if err != nil {
		return validationTranslate(err.(validator.ValidationErrors))
	}
	return []Error{}
}
