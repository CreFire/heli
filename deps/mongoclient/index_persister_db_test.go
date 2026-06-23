package mongoclient_test

import (
	"context"
	"testing"

	"game/deps/mongoclient"
	"game/src/proto/pb"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func insertPlayerBase(t *testing.T, ctx context.Context, collection *mongo.Collection, id, gid int64, name string, level int32) {
	t.Helper()
	_, err := collection.InsertOne(ctx, bson.M{
		"_id":      id,
		"gid":      gid,
		"Nickname": name,
		"level":    level,
	})
	require.NoError(t, err)
}

func TestIndexPersister_LoadLimitAndOrder(t *testing.T) {
	ctx, collection, cleanup := setupTestDB(t)
	defer cleanup()

	gid := int64(1001)
	insertPlayerBase(t, ctx, collection, 1, gid, "n1", 1)
	insertPlayerBase(t, ctx, collection, 2, gid, "n2", 2)
	insertPlayerBase(t, ctx, collection, 3, gid, "n3", 3)

	persister := mongoclient.NewIndexPersister[*pb.PlayerBase](&pb.PlayerBase{}, "gid", collection, gid, 2)
	docs, err := persister.LoadByIndex(gid)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	require.Equal(t, "n3", docs[0].Nickname)
	require.Equal(t, "n2", docs[1].Nickname)

	data := persister.Data()
	require.Len(t, data, 2)
	_, ok := data[int64(3)]
	require.True(t, ok)
	_, ok = data[int64(2)]
	require.True(t, ok)
}

func TestIndexPersister_SaveAllDoc(t *testing.T) {
	ctx, collection, cleanup := setupTestDB(t)
	defer cleanup()

	gid := int64(3001)
	persister := mongoclient.NewIndexPersister[*pb.PlayerBase](&pb.PlayerBase{}, "gid", collection, gid, 200)

	data := map[int64]*pb.PlayerBase{
		5: {Gid: gid, Nickname: "n5"},
		6: {Gid: gid, Nickname: "n6"},
	}
	persister.SetData(data)

	err := persister.Save()
	require.NoError(t, err)

	count, err := collection.CountDocuments(ctx, bson.M{"gid": gid})
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestIndexPersister_DataLazyLoad(t *testing.T) {
	ctx, collection, cleanup := setupTestDB(t)
	defer cleanup()

	gid := int64(6001)
	insertPlayerBase(t, ctx, collection, 1, gid, "lazy", 7)

	persister := mongoclient.NewIndexPersister[*pb.PlayerBase](&pb.PlayerBase{}, "gid", collection, gid, 200)
	data := persister.Data()
	require.Len(t, data, 1)

	doc, ok := data[int64(1)]
	require.True(t, ok)
	require.Equal(t, gid, doc.Gid)
	require.Equal(t, "lazy", doc.Nickname)
}

func TestIndexPersister_ReadOnlySkipSave(t *testing.T) {
	ctx, collection, cleanup := setupTestDB(t)
	defer cleanup()

	gid := int64(4001)
	insertPlayerBase(t, ctx, collection, 1, gid, "old", 1)

	persister := mongoclient.NewIndexPersister[*pb.PlayerBase](&pb.PlayerBase{}, "gid", collection, gid, 200)
	_, err := persister.LoadByIndex(gid)
	require.NoError(t, err)

	persister.SetMode(mongoclient.PersisterModeReadOnly)
	persister.AddUpdateOp(1, &pb.PlayerBase{Gid: gid, Nickname: "new", Lv: 9})
	err = persister.Save()
	require.NoError(t, err)

	var doc bson.M
	err = collection.FindOne(ctx, bson.M{"_id": int64(1)}).Decode(&doc)
	require.NoError(t, err)
	require.Equal(t, "old", doc["Nickname"])
	require.EqualValues(t, 1, doc["level"])
}

func TestIndexPersister_LoadRejectsNonInt64ID(t *testing.T) {
	ctx, collection, cleanup := setupTestDB(t)
	defer cleanup()

	gid := int64(5001)
	_, err := collection.InsertOne(ctx, bson.M{
		"_id":      "bad",
		"gid":      gid,
		"Nickname": "bad",
		"level":    int32(1),
	})
	require.NoError(t, err)

	persister := mongoclient.NewIndexPersister[*pb.PlayerBase](&pb.PlayerBase{}, "gid", collection, gid, 200)
	_, err = persister.LoadByIndex(gid)
	require.Error(t, err)
}

func TestIndexPersister_NoDefaultLoad_GetAndSaveNew(t *testing.T) {
	ctx, collection, cleanup := setupTestDB(t)
	defer cleanup()

	gid := int64(7001)
	insertPlayerBase(t, ctx, collection, 1, gid, "old", 1)

	persister := mongoclient.NewIndexPersister[*pb.PlayerBase](&pb.PlayerBase{}, "gid", collection, gid, 200)
	persister.SetMode(mongoclient.PersisterModeNoDefaultLoad)

	data := persister.Data()
	require.Len(t, data, 0)
	require.False(t, persister.IsLoaded())

	doc, err := persister.Get(1)
	require.NoError(t, err)
	require.Equal(t, "old", doc.Nickname)

	persister.AddUpdateOp(2, &pb.PlayerBase{Gid: gid, Nickname: "new", Lv: 2})
	err = persister.Save()
	require.NoError(t, err)

	var doc2 bson.M
	err = collection.FindOne(ctx, bson.M{"_id": int64(2)}).Decode(&doc2)
	require.NoError(t, err)
	require.Equal(t, "new", doc2["Nickname"])

	var doc1 bson.M
	err = collection.FindOne(ctx, bson.M{"_id": int64(1)}).Decode(&doc1)
	require.NoError(t, err)
	require.Equal(t, "old", doc1["Nickname"])
}
