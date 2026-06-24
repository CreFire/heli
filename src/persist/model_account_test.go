package persist

import (
	"context"
	"testing"
	"time"

	"game/deps/mongoclient"
	"game/deps/server"
	"game/src/configdoc"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func setupAccountTestDB(t *testing.T) (context.Context, *mongo.Collection, func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		cancel()
		t.Skipf("skip account db test: connect mongo failed: %v", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		cancel()
		t.Skipf("skip account db test: ping mongo failed: %v", err)
	}

	dbName := "heli_persist_account_test"
	db := client.Database(dbName)
	coll := db.Collection(COLLECTION_AUTH_ACCOUNT)
	if err := coll.Drop(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		cancel()
		t.Fatalf("drop auth account collection: %v", err)
	}

	oldMS := server.MS
	server.MS = &server.Server{
		MongoDB: &mongoclient.MongoClient{Clients: client},
		ConfBase: &configdoc.ConfigBase{
			Global: &configdoc.GlobalCfg{
				Mongo: &configdoc.MongoCfg{DbName: dbName},
			},
		},
	}

	cleanup := func() {
		_ = coll.Drop(context.Background())
		server.MS = oldMS
		_ = client.Disconnect(context.Background())
		cancel()
	}
	return ctx, coll, cleanup
}

func TestGetAccountWithPasswordCreatesAccountWhenMissing(t *testing.T) {
	ctx, coll, cleanup := setupAccountTestDB(t)
	defer cleanup()

	acc, err := GetAccountWithPassword(ctx, "con_riot_1001", "admin123.")
	if err != nil {
		t.Fatalf("GetAccountWithPassword returned error: %v", err)
	}
	if acc == nil {
		t.Fatalf("expected account to be created")
	}
	if acc.Account != "con_riot_1001" {
		t.Fatalf("unexpected account: %q", acc.Account)
	}
	if acc.Password != "admin123." {
		t.Fatalf("unexpected password: %q", acc.Password)
	}
	if acc.Roles == nil || len(acc.Roles) != 0 {
		t.Fatalf("expected empty roles map, got %#v", acc.Roles)
	}

	count, err := coll.CountDocuments(ctx, bson.M{"account": "con_riot_1001"})
	if err != nil {
		t.Fatalf("count account documents: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one account document, got %d", count)
	}
}

func TestGetAccountWithPasswordRejectsWrongPassword(t *testing.T) {
	ctx, _, cleanup := setupAccountTestDB(t)
	defer cleanup()

	acc := NewAccount("con_riot_1002", time.Now().Unix())
	acc.Password = "right-password"
	if err := CreateAccount(ctx, acc); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	got, err := GetAccountWithPassword(ctx, "con_riot_1002", "wrong-password")
	if err == nil {
		t.Fatalf("expected password error")
	}
	if got != nil {
		t.Fatalf("expected nil account on password error, got %#v", got)
	}
}
