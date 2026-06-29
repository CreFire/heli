package mongoclient

import (
	"testing"

	"game/src/proto/pb"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDataPersisterAddUpdateOpRejectsNonMapNestedPath(t *testing.T) {
	persister := NewDataPersister(&pb.PlayerBase{}, "base", nil, 1)
	persister.SetLoaded()

	persister.AddUpdateOp("level.1", int32(2))

	require.Nil(t, persister.SaveDocs())
}

func TestDataPersisterAddUpdateOpAllowsMapNestedPath(t *testing.T) {
	persister := NewDataPersister(&pb.GamerPackData{}, "pack", nil, 1)
	persister.SetLoaded()

	item := &pb.Item{ConfId: 1, Num: 2}
	persister.AddUpdateOp("cu.1", item)

	docs := persister.SaveDocs()
	require.Equal(t, item, docs["$set"].(bson.M)["pack.cu.1"])
}

func TestDataPersisterAddUnsetOpRejectsNonMapNestedPath(t *testing.T) {
	persister := NewDataPersister(&pb.PlayerBase{}, "base", nil, 1)
	persister.SetLoaded()

	persister.AddUnsetOp("level.1")

	require.Nil(t, persister.SaveDocs())
}
