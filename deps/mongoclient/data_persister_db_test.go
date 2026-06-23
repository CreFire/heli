package mongoclient_test

import (
	"testing"

	"game/deps/mongoclient"
	"game/src/proto/pb"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDataPersister_ReadOnlySkipsSave(t *testing.T) {
	ctx, collection, cleanup := setupTestDB(t)
	defer cleanup()

	gid := int64(9001)
	_, err := collection.InsertOne(ctx, bson.M{
		"_id": int64(1),
		"base": bson.M{
			"gid":      gid,
			"niceName": "old",
			"level":    int32(1),
		},
	})
	require.NoError(t, err)

	persister := mongoclient.NewDataPersister(&pb.PlayerBase{}, "base", collection, 1)
	err = persister.Load()
	require.NoError(t, err)

	persister.SetMode(mongoclient.PersisterModeReadOnly)
	persister.AddUpdateOp("niceName", "new")
	err = persister.Save()
	require.NoError(t, err)

	var doc struct {
		Base bson.M `bson:"base"`
	}
	err = collection.FindOne(ctx, bson.M{"_id": int64(1)}).Decode(&doc)
	require.NoError(t, err)
	require.Equal(t, "old", doc.Base["niceName"])
}
