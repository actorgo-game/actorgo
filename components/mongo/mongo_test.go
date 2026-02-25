package cmongo

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	clog "github.com/actorgo-game/actorgo/logger"
)

type Student struct {
	ID   int32  `bson:"id,omitempty"`
	Name string `bson:"name,omitempty"`
	Age  int    `bson:"age,omitempty"`
}

func TestConnect(t *testing.T) {
	clog.Info("test connect mongodb")

	uri := "mongodb://localhost:27017"
	dbName := "test"

	mdb, err := CreateDatabase(uri, dbName)
	if err != nil {
		clog.Warn(err.Error())
		return
	}

	collection := mdb.Collection("numbers")

	student := &Student{
		ID:   1,
		Name: "aaa111",
		Age:  111,
	}

	uniqueKey := mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}

	_, err = collection.Indexes().CreateOne(context.TODO(), uniqueKey)
	if err != nil {
		clog.Warn(err.Error())
	}

	filter := bson.D{{Key: "id", Value: student.ID}}
	opts := options.FindOneAndUpdate().SetUpsert(true)

	update := bson.D{{Key: "$set", Value: student}}
	ret := collection.FindOneAndUpdate(context.TODO(), filter, update, opts)
	clog.Info("err = %v", ret.Err())

	//replaceID := ret.UpsertedID.(bson.ObjectID)
	findResult := collection.FindOne(context.Background(), filter)

	findStudent := Student{}
	findResult.Decode(&findStudent)
	fmt.Println(findStudent)
}
