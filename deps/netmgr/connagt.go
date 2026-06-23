package netmgr

import (
	"fmt"
	"game/deps/xlog"
	"sync/atomic"
)

type kind uint8

const (
	kNone kind = iota
	kClient
	kServer
)

type routeEnt struct {
	svrType string
	svrId   int32
}

// connSnap 是“不可变快照”，一旦发布（Store/CAS 成功）就永远不再修改。
type connSnap struct {
	k kind

	// client
	gid    int64
	remote string
	routes []routeEnt // 典型 2~3 项；读时线性扫

	// server
	svrType string
	svrId   int32
}

type ConnAgt struct {
	st atomic.Pointer[connSnap]
}

// ------------------------ util ------------------------

func cloneSnap(old *connSnap) *connSnap {
	if old == nil {
		return &connSnap{k: kNone}
	}
	n := new(connSnap)
	*n = *old // shallow copy

	// deep copy slice
	if len(old.routes) > 0 {
		n.routes = make([]routeEnt, len(old.routes))
		copy(n.routes, old.routes)
	}
	return n
}

func (sm *ConnAgt) load() *connSnap {
	if sm == nil {
		return nil
	}
	return sm.st.Load()
}

// ------------------------ String ------------------------

func (sm *ConnAgt) String() string {
	p := sm.load()
	if p == nil {
		return ""
	}
	switch p.k {
	case kClient:
		return fmt.Sprintf("c[%v-%v]", p.gid, p.remote)
	case kServer:
		return fmt.Sprintf("s[%v-%v]", p.svrType, p.svrId)
	default:
		return ""
	}
}

// ------------------------ Server ops ------------------------

func (sm *ConnAgt) AddSvrAgt(svrType string, svrId int32) {
	if sm == nil {
		return
	}
	for {
		old := sm.st.Load()
		if old != nil && old.gid > 0 {
			xlog.Errorf("[ConnAgt] AddSvrAgt: already set gid: %v ", old)
			return
		}
		n := &connSnap{
			k:       kServer,
			svrType: svrType,
			svrId:   svrId,
		}
		if sm.st.CompareAndSwap(old, n) {
			return
		}
	}
}

func (sm *ConnAgt) GetSvrAgt() (string, int32) {
	p := sm.load()
	if p == nil || p.k != kServer {
		return "", -1
	}
	return p.svrType, p.svrId
}

func (sm *ConnAgt) DelSvrSess(f func(string, int32)) {
	if sm == nil || f == nil {
		return
	}
	p := sm.st.Load()
	if p == nil || p.k != kServer {
		return
	}
	f(p.svrType, p.svrId)
}

// ------------------------ Client ops ------------------------

func (sm *ConnAgt) AddCltUser(gid int64) {
	if sm == nil {
		return
	}
	for {
		old := sm.st.Load()
		n := cloneSnap(old)
		// 切到 client：清空 server 信息
		n.k = kClient
		n.svrType, n.svrId = "", 0

		n.gid = gid
		if sm.st.CompareAndSwap(old, n) {
			return
		}
	}
}

func (sm *ConnAgt) GetCltUser() int64 {
	p := sm.load()
	if p == nil || p.k != kClient {
		return -1
	}
	return p.gid
}

func (sm *ConnAgt) AddCltRemote(addr string) {
	if sm == nil {
		return
	}
	for {
		old := sm.st.Load()
		n := cloneSnap(old)
		n.k = kClient
		n.svrType, n.svrId = "", 0

		n.remote = addr
		if sm.st.CompareAndSwap(old, n) {
			return
		}
	}
}

func (sm *ConnAgt) GetCltRemote() string {
	p := sm.load()
	if p == nil || p.k != kClient {
		return ""
	}
	return p.remote
}

// route：只有 2~3 项，线性扫即可；写很少，COW copy slice
func (sm *ConnAgt) AddCltRoute(svrType string, svrId int32) {
	if sm == nil {
		return
	}
	for {
		old := sm.st.Load()
		n := cloneSnap(old)
		n.k = kClient
		n.svrType, n.svrId = "", 0

		// 更新或追加
		updated := false
		for i := range n.routes {
			if n.routes[i].svrType == svrType {
				n.routes[i].svrId = svrId
				updated = true
				break
			}
		}
		if !updated {
			n.routes = append(n.routes, routeEnt{svrType: svrType, svrId: svrId})
		}

		if sm.st.CompareAndSwap(old, n) {
			return
		}
	}
}

func (sm *ConnAgt) GetCltRoute(svrType string) int32 {
	p := sm.load()
	if p == nil || p.k != kClient {
		return -1
	}
	// 2~3 项：线性扫通常比 map 更快/更稳
	for i := range p.routes {
		if p.routes[i].svrType == svrType {
			return p.routes[i].svrId
		}
	}
	return -1
}

func (sm *ConnAgt) DelCltSess(f func(int64, string, int32)) {
	if sm == nil || f == nil {
		return
	}
	p := sm.st.Load()
	if p == nil || p.k != kClient {
		return
	}
	for i := range p.routes {
		f(p.gid, p.routes[i].svrType, p.routes[i].svrId)
	}
}

// ------------------------ ctor ------------------------

func newConnAgt() *ConnAgt {
	sm := &ConnAgt{}
	// 可选：初始化为 kNone，避免 nil 判断分支（也可以不存，让 st.Load() 返回 nil）
	//sm.st.Store(&connSnap{k: kNone})
	return sm
}
