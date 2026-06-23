package configdoc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGlobalCfgApplyEnv(t *testing.T) {
	t.Setenv("MONGO_DSN", "mongodb://127.0.0.1:27017/game_from_env?replicaSet=rs0")
	t.Setenv("REDIS_DSN", "redis://127.0.0.1:6379/9")
	t.Setenv("ETCD_DSN", "etcd://127.0.0.1:2379")

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "global.yaml"), []byte(`
mongo:
  dsn: "mongodb://127.0.0.1:27017/game_from_yaml"
  dbName: "yaml_db"
redisDsn: "redis://127.0.0.1:6379/1"
etcdDsn: "etcd://127.0.0.1:12379"
gameDataPath: "../../docconf"
`), 0o644)
	if err != nil {
		t.Fatalf("write global.yaml: %v", err)
	}

	cfg, err := LoadGlobalCfg(dir)
	if err != nil {
		t.Fatalf("LoadGlobalCfg: %v", err)
	}

	if cfg.Mongo.Dsn != "mongodb://127.0.0.1:27017/game_from_env?replicaSet=rs0" {
		t.Fatalf("unexpected mongo dsn: %s", cfg.Mongo.Dsn)
	}
	if cfg.Mongo.DbName != "game_from_env" {
		t.Fatalf("unexpected mongo dbName: %s", cfg.Mongo.DbName)
	}
	if cfg.RedisDsn != "redis://127.0.0.1:6379/9" {
		t.Fatalf("unexpected redis dsn: %s", cfg.RedisDsn)
	}
	if cfg.EtcdDsn != "etcd://127.0.0.1:2379" {
		t.Fatalf("unexpected etcd dsn: %s", cfg.EtcdDsn)
	}
}

func TestLoadGlobalCfgKeepYamlMongoDbNameWhenDSNHasNoDatabase(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "global.yaml"), []byte(`
mongo:
  dsn: "mongodb://127.0.0.1:27017"
  dbName: "yaml_db"
gameDataPath: "../../docconf"
`), 0o644)
	if err != nil {
		t.Fatalf("write global.yaml: %v", err)
	}

	cfg, err := LoadGlobalCfg(dir)
	if err != nil {
		t.Fatalf("LoadGlobalCfg: %v", err)
	}
	if cfg.Mongo.DbName != "yaml_db" {
		t.Fatalf("unexpected mongo dbName: %s", cfg.Mongo.DbName)
	}
}
