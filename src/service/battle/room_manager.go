package main

import (
	"fmt"
	"sync"

	battlesync "game/src/service/battle/sync"
)

type battleRoom struct {
	id           string
	playerIDs    []int64
	room         *battlesync.Room
	allowedToken string
	joinedSess   map[int64]int64
}

type roomManager struct {
	mu    sync.RWMutex
	rooms map[string]*battleRoom
}

func newRoomManager() *roomManager {
	return &roomManager{
		rooms: make(map[string]*battleRoom),
	}
}

func (m *roomManager) createRoom(roomID string, playerIDs []int64, towerDeck []int32, battleToken string) (*battleRoom, error) {
	if roomID == "" {
		return nil, fmt.Errorf("room id is empty")
	}
	if len(playerIDs) == 0 {
		return nil, fmt.Errorf("player ids is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if room := m.rooms[roomID]; room != nil {
		return nil, fmt.Errorf("room already exists")
	}

	cfg := battlesync.RoomConfig{
		InitialGold:      100,
		InitialMana:      0,
		BuildCostGold:    10,
		RerollCostMana:   1,
		MinerCostGold:    20,
		MinerProduceMana: 1,
		TowerMaxLevel:    5,
		DefaultTowerDeck: append([]int32(nil), towerDeck...),
	}
	if len(cfg.DefaultTowerDeck) == 0 {
		cfg.DefaultTowerDeck = []int32{101}
	}

	room := battlesync.NewRoom(cfg)
	for _, playerID := range playerIDs {
		if err := room.AddPlayer(playerID, towerDeck); err != nil {
			return nil, err
		}
	}

	ret := &battleRoom{
		id:           roomID,
		playerIDs:    append([]int64(nil), playerIDs...),
		room:         room,
		allowedToken: battleToken,
		joinedSess:   make(map[int64]int64, len(playerIDs)),
	}
	m.rooms[roomID] = ret
	return ret, nil
}

func (m *roomManager) getRoom(roomID string) *battleRoom {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rooms[roomID]
}

func (m *roomManager) roomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}

func (r *battleRoom) hasPlayer(playerID int64) bool {
	for _, id := range r.playerIDs {
		if id == playerID {
			return true
		}
	}
	return false
}

func (r *battleRoom) bindPlayerSession(playerID, sessID int64) {
	if r == nil {
		return
	}
	if r.joinedSess == nil {
		r.joinedSess = map[int64]int64{}
	}
	r.joinedSess[playerID] = sessID
}

func (r *battleRoom) matchPlayerSession(playerID, sessID int64) bool {
	if r == nil {
		return false
	}
	bound, ok := r.joinedSess[playerID]
	return ok && bound == sessID
}
