package configdoc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BattleDoc holds all battle-related configuration loaded from docconf/battle/.
type BattleDoc struct {
	Enemies        []map[string]any // battle_enemies.json
	Towers         []map[string]any // battle_towers.json
	Bosses         []map[string]any // battle_bosses.json
	BattleSettings []map[string]any // battle_battle_settings.json
}

// GetSetting looks up a battle setting value by key. Returns nil if not found.
func (b *BattleDoc) GetSetting(key string) any {
	for _, s := range b.BattleSettings {
		if v, ok := s["key"]; ok && v == key {
			return s["value"]
		}
	}
	return nil
}

// loadBattleTables reads battle JSON files from confDir/battle/.
func loadBattleTables(confDir string) (*BattleDoc, error) {
	battleDir := filepath.Join(confDir, "battle")
	doc := &BattleDoc{}

	load := func(filename string) ([]map[string]any, error) {
		path := filepath.Join(battleDir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read battle file %s: %v", path, err)
		}
		var result []map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse battle JSON %s: %v", filename, err)
		}
		return result, nil
	}

	var err error
	if doc.Enemies, err = load("battle_enemies.json"); err != nil {
		return nil, err
	}
	if doc.Towers, err = load("battle_towers.json"); err != nil {
		return nil, err
	}
	if doc.Bosses, err = load("battle_bosses.json"); err != nil {
		return nil, err
	}
	if doc.BattleSettings, err = load("battle_battle_settings.json"); err != nil {
		return nil, err
	}
	return doc, nil
}
