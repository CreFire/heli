package gateuser

import (
	"errors"
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/xlist"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/common"

	"github.com/sasha-s/go-deadlock"
)

type SendMessageInfo struct {
	SendTime int64
	*msg.Message
}

const (
	CacheTimeout            = 2 * 60
	reconnectShortWindowSec = int64(60)
	reconnectShortWindowMax = 5
	tokenUsedMaxTimes       = 10
	clientRequestWindowMs   = int64(5 * 1000)
	clientBurstWindowMs     = int64(1000)
	clientRequestLimit      = 15
	clientBurstLimit        = 8
)

type GateUserStatus int

const (
	UserStatusOffline GateUserStatus = iota
	UserStatusOnline  GateUserStatus = iota
	UserStatusLoading
	UserStatusLoginOk
)

type GateUser struct {
	GamerId          int64
	Seq              uint16                       // 发送序号
	Ack              uint16                       // 接收序号
	SendQueue        *xlist.List[SendMessageInfo] // 待确认发送消息队列，只保留2分钟内的消息
	SessId           int64                        // sessionId
	LogoutReasons    map[int64]string             // 按session记录待消费的登出原因
	RecentReconnects []int64                      // 记录最近重连时间点
	ReconnectCount   int32
	ClientReqHead    int
	ClientReqCount   int
	ClientReqTimes   [clientRequestLimit]int64
	RWMutex          *deadlock.RWMutex
}

func NewGateUser(gamerId int64, SessId int64) *GateUser {
	return &GateUser{
		GamerId:       gamerId,
		RWMutex:       &deadlock.RWMutex{},
		SendQueue:     xlist.New[SendMessageInfo](),
		SessId:        SessId,
		LogoutReasons: make(map[int64]string),
	}
}

func (g *GateUser) IncSeqAndPullAck() (uint16, uint16) {
	g.Seq++
	return g.Seq, g.Ack
}

func (g *GateUser) AddSendMessage(msg *msg.Message) {
	now := xtime.NowUnix()
	g.SendQueue.Push(SendMessageInfo{Message: msg, SendTime: now})
	node := g.SendQueue.Front()

	for node != nil {
		if node.Value.SendTime+common.GATE_CACHE_USER_TIME < now {
			old := node
			node = node.Next()
			g.SendQueue.Remove(old)
		} else {
			break
		}
	}
}

func (g *GateUser) UpdateAck(cseq, cack uint16) {
	node := g.SendQueue.Front()
	now := xtime.NowUnix()
	g.Ack = cseq
	for node != nil {
		if IsSeqGreater(cack, uint16(node.Value.Head.Seq)) || node.Value.SendTime+common.GATE_CACHE_USER_TIME < now {
			old := node
			node = node.Next()
			g.SendQueue.Remove(old)
		} else {
			break
		}
	}
}
func (g *GateUser) GetAllSendMessage() *xlist.List[SendMessageInfo] {
	return g.SendQueue
}

// CanReconnect 检查是否可以重连，仅做短窗口频率限制和token最大使用次数限制
func (g *GateUser) CanReconnect() bool {
	g.RWMutex.Lock()
	defer g.RWMutex.Unlock()

	g.ReconnectCount++
	if g.ReconnectCount > tokenUsedMaxTimes {
		return false
	}

	now := xtime.NowUnix()
	// 清理超过时间窗口的重连记录（1分钟）
	g.RecentReconnects = append(g.RecentReconnects, now)
	newRecentReconnects := make([]int64, 0, len(g.RecentReconnects))
	for _, t := range g.RecentReconnects {
		if now-t < reconnectShortWindowSec {
			newRecentReconnects = append(newRecentReconnects, t)
		}
	}

	// 检查时间窗口内的重连次数是否超过限制
	if len(newRecentReconnects) >= reconnectShortWindowMax {
		return false
	}

	return true
}

// GetGamerId 获取玩家ID
func (g *GateUser) GetGamerId() int64 {
	return g.GamerId
}

// GetSessId 获取会话ID
func (g *GateUser) GetSessId() int64 {
	g.RWMutex.RLock()
	defer g.RWMutex.RUnlock()

	return g.SessId
}

func (g *GateUser) MatchSessId(sessId int64) bool {
	g.RWMutex.RLock()
	defer g.RWMutex.RUnlock()

	return g.SessId == sessId
}

// SetSessId 设置会话ID
func (g *GateUser) SetSessId(sessId int64) {
	g.RWMutex.Lock()
	defer g.RWMutex.Unlock()
	g.SessId = sessId
}

func (g *GateUser) SetLogoutReason(sessId int64, reason string) {
	g.RWMutex.Lock()
	defer g.RWMutex.Unlock()
	if sessId <= 0 || reason == "" {
		return
	}
	if g.LogoutReasons == nil {
		g.LogoutReasons = make(map[int64]string)
	}
	g.LogoutReasons[sessId] = reason
}

func (g *GateUser) TakeLogoutReason(sessId int64) string {
	g.RWMutex.Lock()
	defer g.RWMutex.Unlock()
	if sessId <= 0 || g.LogoutReasons == nil {
		return ""
	}
	reason := g.LogoutReasons[sessId]
	delete(g.LogoutReasons, sessId)
	return reason
}

func (g *GateUser) SnapshotLogoutReasons() map[int64]string {
	g.RWMutex.RLock()
	defer g.RWMutex.RUnlock()
	if len(g.LogoutReasons) == 0 {
		return nil
	}
	reasons := make(map[int64]string, len(g.LogoutReasons))
	for sessId, reason := range g.LogoutReasons {
		reasons[sessId] = reason
	}
	return reasons
}

func (g *GateUser) TryAcceptClientRequest() bool {
	g.RWMutex.Lock()
	defer g.RWMutex.Unlock()

	return g.tryAcceptClientRequestLocked()
}

func (g *GateUser) tryAcceptClientRequestLocked() bool {
	now := xtime.NowUnixMs()

	for g.ClientReqCount > 0 {
		oldest := g.ClientReqTimes[g.ClientReqHead]
		if now-oldest < clientRequestWindowMs {
			break
		}
		g.ClientReqHead = (g.ClientReqHead + 1) % clientRequestLimit
		g.ClientReqCount--
	}

	if g.ClientReqCount >= clientRequestLimit {
		return false
	}

	burstCount := 0
	for i := 0; i < g.ClientReqCount; i++ {
		idx := (g.ClientReqHead + g.ClientReqCount - 1 - i + clientRequestLimit) % clientRequestLimit
		if now-g.ClientReqTimes[idx] >= clientBurstWindowMs {
			break
		}
		burstCount++
	}
	if burstCount >= clientBurstLimit {
		return false
	}

	tail := (g.ClientReqHead + g.ClientReqCount) % clientRequestLimit
	g.ClientReqTimes[tail] = now
	g.ClientReqCount++
	return true
}

func (g *GateUser) CheckAndUpdateSeqAckAndLimit(sessId int64, cseq uint16, cack uint16) (bool, error) {
	g.RWMutex.Lock()
	defer g.RWMutex.Unlock()

	if err := g.checkAndUpdateAckSeqLocked(sessId, cseq, cack); err != nil {
		return false, err
	}
	return !g.tryAcceptClientRequestLocked(), nil
}

// CheckAndUpdateAckSeq 检查并更新Ack序列号
func (g *GateUser) CheckAndUpdateAckSeq(sessId int64, cseq uint16, cack uint16) error {
	g.RWMutex.Lock()
	defer g.RWMutex.Unlock()

	return g.checkAndUpdateAckSeqLocked(sessId, cseq, cack)
}

func (g *GateUser) checkAndUpdateAckSeqLocked(sessId int64, cseq uint16, cack uint16) error {
	if g.SessId != sessId {
		if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
			xlog.Debugf("gate user sess mismatch: gid=%v sess=%v user.sess=%v", g.GamerId, sessId, g.SessId)
		}
		return errors.New("gate user sess mismatch")
	}
	if !IsSeqGreater(cseq, g.Ack) {
		if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
			xlog.Debugf("gate user seq not greater: gid=%v sess=%v cseq=%v ack=%v", g.GamerId, sessId, cseq, g.Ack)
		}
		return errors.New("gate user seq not greater")
	}

	g.UpdateAck(cseq, cack)
	return nil
}

// AddSendMessageSeqAck 添加发送消息的Seq 和 Ack
func (g *GateUser) AddSendMessageSeqAck(sessId int64, data *msg.Message) bool {
	g.RWMutex.Lock()
	defer g.RWMutex.Unlock()

	if g.SessId != sessId {
		if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
			xlog.Debugf("gate user add send sess mismatch: gid=%v sess=%v user.sess=%v msgId=%v", g.GamerId, sessId, g.SessId, data.MsgId())
		}
		return false
	}

	seq, ack := g.IncSeqAndPullAck()
	data.SetSeq(int32(seq)).SetAck(int32(ack))
	g.AddSendMessage(data)
	return true
}

// ResendUnAckMessages 重发未确认的消息
func (g *GateUser) ResendUnAckMessages(msgque netmgr.IMsgQue, reqAck uint16) {
	g.RWMutex.RLock()
	defer g.RWMutex.RUnlock()

	li := g.GetAllSendMessage()
	for node := li.Front(); node != nil; node = node.Next() {
		if IsSeqGreater(uint16(node.Value.Head.Seq), reqAck) {
			msgque.Send(node.Value.Message)
		}
	}
	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		xlog.Debugf("resend unacked messages gid:%d sessId:%d reqAck:%d", g.GamerId, msgque.SessId(), reqAck)
	}
}

func IsSeqGreater(s1 uint16, s2 uint16) bool {
	return (s1 > s2 && s1-s2 < 32768) || (s1 < s2 && s2-s1 > 32768)
}
