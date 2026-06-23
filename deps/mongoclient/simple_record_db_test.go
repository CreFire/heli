package mongoclient_test

import (
	"maps"
	"testing"

	"game/deps/mongoclient"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestPlayerDataWrapper is a wrapper struct because SimpleRecorder operates on a parent field.
type TestPlayerDataWrapper struct {
	ID         string      `bson:"_id"`
	PlayerData *PlayerData `bson:"playerData"`
}

func TestSimpleRecorder_DBSync(t *testing.T) {
	ctx, collection, cleanup := setupTestDB(t)
	defer cleanup()

	// --- Initial State ---
	initialPlayer := &PlayerData{
		Profile: &Profile{
			Name:  "InitialName",
			Level: 1,
			Data:  bson.M{"x": int32(1)}, // MongoDB驱动会将int转换为int32
		},
		Items: map[string]int{
			"potion": 5,
			"sword":  1,
		},
		Status: "active",
	}

	initialDoc := &TestPlayerDataWrapper{
		ID:         "player1",
		PlayerData: initialPlayer,
	}

	_, err := collection.InsertOne(ctx, initialDoc)
	require.NoError(t, err)

	t.Run("MixedSetAndUnset", func(t *testing.T) {
		// Keep a copy of the initial state for in-memory modification
		expectedPlayer := *initialPlayer
		expectedPlayer.Items = make(map[string]int)
		maps.Copy(expectedPlayer.Items, initialPlayer.Items)

		// --- Operations ---
		sr := mongoclient.NewSingleDocRecorder("playerData")

		// 1. Set a top-level field
		err := sr.SetField("status", "inactive")
		require.NoError(t, err)
		expectedPlayer.Status = "inactive"

		// 2. Set a field in a nested struct
		err = sr.SetField("profile.name", "NewName")
		require.NoError(t, err)
		expectedPlayer.Profile.Name = "NewName"

		// 3. Set a value in a map
		err = sr.SetField("items.gold", 100)
		require.NoError(t, err)
		expectedPlayer.Items["gold"] = 100

		// 4. Unset a value from a map
		err = sr.UnsetField("items.potion")
		require.NoError(t, err)
		delete(expectedPlayer.Items, "potion")

		// --- DB Application and Verification ---
		updateDoc := sr.GenUpdateDoc()
		require.NotNil(t, updateDoc)

		_, err = collection.UpdateOne(ctx, bson.M{"_id": initialDoc.ID}, updateDoc)
		require.NoError(t, err)

		var finalDoc TestPlayerDataWrapper
		err = collection.FindOne(ctx, bson.M{"_id": initialDoc.ID}).Decode(&finalDoc)
		require.NoError(t, err)

		// Compare the sub-document from DB with the in-memory expected version
		require.Equal(t, &expectedPlayer, finalDoc.PlayerData)
	})

	t.Run("SaveAll_ReplacesSubDocument", func(t *testing.T) {
		// Reset document to initial state for this test case
		_, err := collection.ReplaceOne(ctx, bson.M{"_id": initialDoc.ID}, initialDoc)
		require.NoError(t, err)

		// --- Operations ---
		sr := mongoclient.NewSingleDocRecorder("playerData")

		// These operations should be ignored
		err = sr.SetField("status", "should_be_overwritten")
		require.NoError(t, err)
		err = sr.UnsetField("items.sword")
		require.NoError(t, err)

		// The new PlayerData that will replace the old one
		replacementPlayer := &PlayerData{
			Status: "replaced",
			Tags:   []string{"new", "fresh"},
			Items:  map[string]int{"gem": 1},
		}
		sr.SaveAll(replacementPlayer)

		// --- DB Application and Verification ---
		updateDoc := sr.GenUpdateDoc()
		require.NotNil(t, updateDoc)

		_, err = collection.UpdateOne(ctx, bson.M{"_id": initialDoc.ID}, updateDoc)
		require.NoError(t, err)

		var finalDoc TestPlayerDataWrapper
		err = collection.FindOne(ctx, bson.M{"_id": initialDoc.ID}).Decode(&finalDoc)
		require.NoError(t, err)

		// The Profile should be nil as it wasn't in the replacement
		require.Nil(t, finalDoc.PlayerData.Profile)
		// Compare the sub-document from DB with the replacement object
		require.Equal(t, replacementPlayer, finalDoc.PlayerData)
	})
}
