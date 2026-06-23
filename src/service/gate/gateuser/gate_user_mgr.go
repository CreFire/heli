package gateuser

import (
	"fmt"
	"game/deps/xlog"

	"github.com/phuslu/shardmap"
	"github.com/sasha-s/go-deadlock"
)

var UserMgr = NewGateUserMgr()

type GateUserMgr struct {
	Map               *shardmap.Map[int64, *GateUser]
	noNeedSeqAckMsgId map[int]struct{}
	mu                deadlock.Mutex
}

func NewGateUserMgr() *GateUserMgr {
	m := shardmap.New[int64, *GateUser](4096)
	return &GateUserMgr{
		Map:               m,
		noNeedSeqAckMsgId: map[int]struct{}{},
	}
}

func (m *GateUserMgr) Init() {

}

func (m *GateUserMgr) Get(gamerId int64) (*GateUser, bool) {
	return m.Map.Get(gamerId)
}

func (m *GateUserMgr) Set(gamerId int64, user *GateUser) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Map.Set(gamerId, user)
}

func (m *GateUserMgr) Del(gamerId int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Map.Delete(gamerId)
}

func (m *GateUserMgr) DelBySess(gamerId int64, sessId int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.Map.Get(gamerId)
	if !ok {
		return false
	}
	user.RWMutex.Lock()
	defer user.RWMutex.Unlock()

	if user.SessId != sessId {
		return false
	}
	m.Map.Delete(gamerId)
	return true
}

func (m *GateUserMgr) DelOffline(gamerId int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.Map.Get(gamerId)
	if !ok {
		return false
	}
	user.RWMutex.Lock()
	defer user.RWMutex.Unlock()

	if user.SessId != 0 {
		return false
	}
	m.Map.Delete(gamerId)
	return true
}

func (m *GateUserMgr) ClearSessBySess(gamerId int64, sessId int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.Map.Get(gamerId)
	if !ok {
		return false
	}
	user.RWMutex.Lock()
	defer user.RWMutex.Unlock()

	if user.SessId != sessId {
		return false
	}
	user.SessId = 0
	return true
}

func (m *GateUserMgr) ReplaceSess(gamerId int64, sessId int64, newSessId int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.Map.Get(gamerId)
	if !ok {
		return false
	}
	user.RWMutex.Lock()
	defer user.RWMutex.Unlock()

	if user.SessId != sessId && user.SessId != 0 {
		return false
	}
	user.SessId = newSessId
	return true
}

func (m *GateUserMgr) SetLogoutReason(gamerId int64, sessId int64, reason string) {
	if gamerId <= 0 || sessId <= 0 || reason == "" {
		return
	}
	user, ok := m.Map.Get(gamerId)
	if !ok || user == nil {
		return
	}
	user.SetLogoutReason(sessId, reason)
}

func (m *GateUserMgr) TakeLogoutReason(gamerId int64, sessId int64) string {
	if gamerId <= 0 || sessId <= 0 {
		return ""
	}
	user, ok := m.Map.Get(gamerId)
	if !ok || user == nil {
		return ""
	}
	return user.TakeLogoutReason(sessId)
}

func (m *GateUserMgr) SetNoNeedSeqAckMsgId(msgId int) {
	m.noNeedSeqAckMsgId[msgId] = struct{}{}
}
func (m *GateUserMgr) NeedSetAckSeq(msgId int) bool {
	_, ok := m.noNeedSeqAckMsgId[msgId]
	return !ok
}

func (m *GateUserMgr) DelGateUser(gamerId int64) (*GateUser, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Map.Delete(gamerId)
}

func (m *GateUserMgr) CheckAndUpdateSeqAck(gamerId int64, sessId int64, cseq uint16, cack uint16) error {
	user, ok := m.Get(gamerId)
	if !ok {
		xlog.Debugf("gate user not found: %v", gamerId)
		return fmt.Errorf("gate user not found: %v", gamerId)
	}

	return user.CheckAndUpdateAckSeq(sessId, cseq, cack)
}

func (m *GateUserMgr) CheckAndUpdateSeqAckAndLimit(gamerId int64, sessId int64, cseq uint16, cack uint16) (bool, error) {
	user, ok := m.Get(gamerId)
	if !ok || user == nil {
		xlog.Debugf("gate user not found: %v", gamerId)
		return false, fmt.Errorf("gate user not found: %v", gamerId)
	}

	return user.CheckAndUpdateSeqAckAndLimit(sessId, cseq, cack)
}

func (m *GateUserMgr) AddGateUser(gamerId int64, SessId int64, forceInit bool) *GateUser {
	m.mu.Lock()
	defer m.mu.Unlock()

	if user, ok := m.Map.Get(gamerId); ok && user != nil {
		if !forceInit {
			return user
		}
		newUser := NewGateUser(gamerId, SessId)
		for pendingSessId, reason := range user.SnapshotLogoutReasons() {
			newUser.LogoutReasons[pendingSessId] = reason
		}
		m.Map.Set(gamerId, newUser)
		xlog.Debugf("reset user gate info , gamerId: %d sessid %d", gamerId, SessId)
		return newUser
	}
	user := NewGateUser(gamerId, SessId)
	m.Map.Set(gamerId, user)
	xlog.Debugf("add user gate info , gamerId: %d sessid %d", gamerId, SessId)
	return user
}

func (m *GateUserMgr) OnlineSessions() map[int64]int64 {
	sessions := make(map[int64]int64, 64)
	m.Map.Range(func(gamerId int64, user *GateUser) bool {
		if user == nil {
			return true
		}
		sessId := user.GetSessId()
		if sessId <= 0 {
			return true
		}
		sessions[gamerId] = sessId
		return true
	})
	return sessions
}
