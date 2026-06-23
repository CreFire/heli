package mongoclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Profile struct {
	Name  string `bson:"name"`
	Level int    `bson:"level"`
	Data  bson.M `bson:"data,omitempty"`
}

type PlayerData struct {
	ID      string         `bson:"_id"`
	Profile *Profile       `bson:"profile"`
	Items   map[string]int `bson:"items"`
	Tags    []string       `bson:"tags"`
	Status  string         `bson:"status"`
}

const (
	testMongoURI       = "mongodb://localhost:27017" // CHANGE IF YOUR MONGO IS ELSEWHERE
	testDBName         = "mongoclient_test_db"
	testCollectionName = "player_records"
)

// setupTestDB connects to mongo, creates a clean collection for the test, and returns it.
// It also returns a cleanup function to be deferred.
func setupTestDB(t *testing.T) (context.Context, *mongo.Collection, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)

	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI))
	require.NoError(t, err, "Failed to connect to MongoDB")

	db := client.Database(testDBName)
	collection := db.Collection(testCollectionName)

	err = collection.Drop(ctx)
	require.NoError(t, err)

	cleanup := func() {
		if err := collection.Drop(context.Background()); err != nil {
			t.Logf("Failed to drop collection: %v", err)
		}
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("Failed to disconnect client: %v", err)
		}
		cancel()
	}

	return ctx, collection, cleanup
}
