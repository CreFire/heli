package configdoc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRobotYamlConfigWithoutGlobalYaml(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "robot.yaml")
	err := os.WriteFile(confPath, []byte(`
type: "robot"
id: 9001
port: 7000
gameDataPath: "../../docconf"
robot:
  count: 10
  loginRate: 5
  auth: "http://127.0.0.1:15000"
log:
  level: "info"
  fileName: "robot"
`), 0o600)
	require.NoError(t, err)

	cfg, err := LoadRobotYamlConfig(confPath)
	require.NoError(t, err)
	require.Equal(t, int32(9001), cfg.Server.Id)
	require.Equal(t, filepath.Clean(filepath.Join(dir, "../../docconf")), cfg.Global.GameDataPath)
	require.Equal(t, "info", cfg.Server.Log.Level)
	require.Equal(t, "robot", cfg.Server.Log.FileName)
}

func TestLoadRobotYamlConfigResolvesGameDataPathRelativeToYaml(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "robot.yaml")
	err := os.WriteFile(confPath, []byte(`
type: "robot"
id: 9001
port: 7000
gameDataPath: "./json"
robot:
  count: 10
  loginRate: 5
  auth: "http://127.0.0.1:15000"
log:
  level: "info"
  fileName: "robot"
`), 0o600)
	require.NoError(t, err)

	cfg, err := LoadRobotYamlConfig(confPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "json"), cfg.Global.GameDataPath)
}
