package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type Collection string
type Operator string
type Type string

const (
	RAW string = "Raw"
)

const (
	LESS          Operator = "<"              // less than
	LESSEQUAL     Operator = "<="             // less than or equal to
	EQUAL         Operator = "=="             //equal to
	GREATER       Operator = ">"              // greater than
	GREATEREQUAL  Operator = ">="             // greater than or equal to
	NOTEQUAL      Operator = "!="             // not equal to
	ARRAYCONTAINS Operator = "array-contains" // array-contains
	//array-contains-any
	//in
	//not-in
)

const (
	STRING  Type = "string"
	BOOLEAN Type = "boolean"
	NUMBER  Type = "number"
	DATE    Type = "timestamp"
)

type Model interface {
	Collection() Collection
	DocId() string
	Validate() []Error
	Searchify() []string
}

type Record struct {
	Id        string
	Created   time.Time
	Updated   time.Time
	CreatedBy string
	UpdatedBy string
	Raw       []string `json:"-"`
}

type Filter struct {
	Field    string
	Operator Operator
	Type     Type
	Value    string
}

type Order struct {
	Field      string
	Descending bool `json:",string"`
}

type Payload struct {
	Page     int `json:",string"`
	PageSize int `json:",string"`
	Search   string
	Filters  []Filter
	Orders   []Order
}

type ResultList[T Model] struct {
	Count int
	Data  []T
}

var storage *firestore.Client

func getCredentials() (option.ClientOption, string, error) {
	accessFile, err := os.Open("./config.json")
	if err != nil {
		return nil, "", err
	}
	defer accessFile.Close()

	jsonFile, err := ioutil.ReadAll(accessFile)
	if err != nil {
		return nil, "", err
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(jsonFile), &result)
	projectId := result["project_id"].(string)

	return option.WithCredentialsJSON(jsonFile), projectId, nil
}

func NewFireStore() {
	ctx := context.Background()

	option, projectId, err := getCredentials()
	if err != nil {
		panic(err)
	}

	storage, err = firestore.NewClient(ctx, projectId, option)
	if err != nil {
		panic(err)
	}
}

func FireStoreClose() {
	if storage != nil {
		storage.Close()
	}
}

func StorageGet[T Model](id string) T {
	var entity T

	collection := storage.Collection(string(entity.Collection()))
	doc, err := collection.Doc(id).Get(context.Background())
	if err != nil {
		log.Println(err)
		return entity
	}

	err = doc.DataTo(&entity)
	if err != nil {
		log.Println(err)
		return entity
	}

	return entity
}

func StorageNewId[T Model]() string {
	var entity T
	return storage.Collection(string(entity.Collection())).NewDoc().ID
}

func StorageCount[T Model](filters []Filter) int {
	var entity T

	collection := storage.Collection(string(entity.Collection()))
	query := collection.Query

	for _, f := range filters {
		switch f.Type {
		case BOOLEAN:
			value, err := strconv.ParseBool(f.Value)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		case NUMBER:
			value, err := strconv.ParseFloat(f.Value, 64)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		case DATE:
			value, err := time.Parse("2006-01-02", f.Value)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		default:
			query = query.Where(f.Field, string(f.Operator), f.Value)
		}
	}

	snap, _ := query.Documents(context.Background()).GetAll()
	count := len(snap)

	return count
}

func StorageSync[T Model](entity T) error {
	doc := storage.Collection(string(entity.Collection())).Doc(entity.DocId())
	reflect.ValueOf(&entity).Elem().FieldByName(RAW).Set(reflect.ValueOf(searchify(entity.Searchify())))
	_, err := doc.Set(context.Background(), entity)
	if err != nil {
		return err
	}

	return nil
}

func StorageSyncList[T Model](payload Payload, field string, value string) {
	var entity T

	collection := storage.Collection(string(entity.Collection()))
	query := collection.Query

	for _, f := range payload.Filters {
		switch f.Type {
		case BOOLEAN:
			value, err := strconv.ParseBool(f.Value)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		case NUMBER:
			value, err := strconv.ParseFloat(f.Value, 64)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		case DATE:
			value, err := time.Parse("2006-01-02", f.Value)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		default:
			query = query.Where(f.Field, string(f.Operator), f.Value)
		}
	}

	snap, _ := query.Documents(context.Background()).GetAll()
	count := len(snap)
	if count == 0 {
		return
	}

	batch := storage.Batch()
	iter := query.Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if doc == nil || err == iterator.Done {
			break
		}

		batch.Set(doc.Ref, map[string]interface{}{
			field: value,
		}, firestore.MergeAll)
	}

	// Commit the batch
	_, err := batch.Commit(context.Background())
	if err != nil {
		log.Printf("An error has occurred: %s", err)
	}
}

func StorageDelete[T Model](entity T) error {
	_, err := storage.Collection(string(entity.Collection())).Doc(entity.DocId()).Delete(context.Background())
	if err != nil {
		return err
	}

	return nil
}

func StorageList[T Model](payload Payload) ResultList[T] {
	var result []T = []T{}
	var entity T

	collection := storage.Collection(string(entity.Collection()))
	query := collection.Query

	for _, f := range payload.Filters {
		switch f.Type {
		case BOOLEAN:
			value, err := strconv.ParseBool(f.Value)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		case NUMBER:
			value, err := strconv.ParseFloat(f.Value, 64)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		case DATE:
			value, err := time.Parse("2006-01-02", f.Value)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		default:
			query = query.Where(f.Field, string(f.Operator), f.Value)
		}
	}

	snap, _ := query.Documents(context.Background()).GetAll()
	count := len(snap)

	for _, o := range payload.Orders {
		sort := firestore.Asc
		if o.Descending {
			sort = firestore.Desc
		}
		query = query.OrderBy(o.Field, sort)
	}

	if payload.PageSize > 0 {
		query = query.Limit(payload.PageSize)
	}

	if payload.Page > 0 {
		query = query.Offset(payload.Page * payload.PageSize)
	}

	iter := query.Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if doc == nil || err == iterator.Done {
			break
		}

		var entity T
		err = doc.DataTo(&entity)
		if err != nil {
			log.Println(err)
			continue
		}

		result = append(result, entity)
	}

	return ResultList[T]{
		Count: count,
		Data:  result,
	}
}

func StorageSum[T Model](filters []Filter, field string) float64 {
	var entity T

	collection := storage.Collection(string(entity.Collection()))
	query := collection.Query

	for _, f := range filters {
		switch f.Type {
		case BOOLEAN:
			value, err := strconv.ParseBool(f.Value)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		case NUMBER:
			value, err := strconv.ParseFloat(f.Value, 64)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		case DATE:
			value, err := time.Parse("2006-01-02", f.Value)
			if err == nil {
				query = query.Where(f.Field, string(f.Operator), value)
			}
		default:
			query = query.Where(f.Field, string(f.Operator), f.Value)
		}
	}

	snap, _ := query.Documents(context.Background()).GetAll()
	count := len(snap)
	if count == 0 {
		return 0
	}

	iter := query.Documents(context.Background())
	defer iter.Stop()

	sum := float64(0)

	for {
		doc, err := iter.Next()
		if doc == nil || err == iterator.Done {
			break
		}

		v := doc.Data()[field]
		str := fmt.Sprintf("%v", v)
		v2, err := strconv.ParseFloat(str, 64)
		if err == nil {
			sum += v2
		}
	}

	return sum
}

// ////////////////////////////////////////////////////////////////////////////
// utils
func searchify(terms []string) []string {
	list := []string{}
	for _, item := range terms {
		if len(item) > 0 {
			item = strings.ToLower(item)
			list = append(list, item)

			// Convert to rune slice for substrings.
			runes := []rune(item)

			// Loop over possible lengths, and possible start indexes.
			// ... Then take each possible substring from the source string.
			for length := 2; length < len(runes); length++ {
				for start := 0; start <= len(runes)-length; start++ {
					substring := string(runes[start : start+length])
					list = append(list, substring)
				}
			}
		}
	}
	return list
}
