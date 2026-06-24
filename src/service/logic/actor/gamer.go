package actor

import (
	"errors"
	"game/deps/msg"
	"game/deps/server"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/configdoc"
	"game/src/persist"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/iface"
	matchmodule "game/src/service/logic/module/matchbiz"
	shopmodule "game/src/service/logic/module/shopbiz"
	taskmodule "game/src/service/logic/module/taskbiz"
	"math/rand"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

var ErrGamerStopped = errors.New("gamer stopped")
var ErrGamerTaskQueueFull = errors.New("gamer task queue full")

const GAMER_MSG_CHAN_SIZE = 128

type GamerStatus int

const (
	GamerStatus_Offline GamerStatus = iota
	GamerStatus_Online
)

type GamerSessState struct {
	gateSessId   int64
	playerSessId int64
	status       GamerStatus
}

type Gamer struct {
	GamerId        int64
	Model          *gamedata.GamerModel
	doc            *configdoc.DocPbConfig
	docExtend      *configdoc.DocExtendConfig
	msgChan        chan func()
	sessState      atomic.Pointer[GamerSessState]
	stopped        atomic.Bool
	lastHeart      int64
	heartFastCount atomic.Int32
	mgr            *GamerManager
	logger         iface.ILogger
	rand           *rand.Rand

	playerModule   iface.IPlayerModule
	heroModule     iface.IHeroModule
	itemModule     iface.IItemModule
	taskModule     iface.ITaskModule
	shopModule     iface.IShopModule
	mailModule     iface.IMailModule
	activityModule iface.IActivityModule
	packModule     iface.IPackModule
	functionModule iface.IFunctionModule
	matchModule    iface.IMatchModule
}

func (r *Gamer) updateSessState(update func(state *GamerSessState)) GamerSessState {
	old := r.sessState.Load()
	next := GamerSessState{}
	if old != nil {
		next = *old
	}
	update(&next)
	r.sessState.Store(&next)
	return next
}

func (r *Gamer) GetGateSessId() int64 {
	if s := r.sessState.Load(); s != nil {
		return s.gateSessId
	}
	return 0
}
func (r *Gamer) GetPlayerSessId() int64 {
	if s := r.sessState.Load(); s != nil {
		return s.playerSessId
	}
	return 0
}
func (r *Gamer) GetSessIds() (int64, int64) {
	if s := r.sessState.Load(); s != nil {
		return s.playerSessId, s.gateSessId
	}
	return 0, 0
}
func (r *Gamer) SetLastHeart(heart int64) { r.lastHeart = heart }
func (r *Gamer) GetLastHeart() int64      { return r.lastHeart }
func (r *Gamer) IncHeartFastCount() int   { return int(r.heartFastCount.Add(1)) }
func (r *Gamer) ResetHeartFastCount()     { r.heartFastCount.Store(0) }
func (r *Gamer) GetGamerId() int64        { return r.GamerId }
func (r *Gamer) SetOnlineStatus(status GamerStatus) {
	r.updateSessState(func(s *GamerSessState) { s.status = status })
}
func (r *Gamer) IsOnline() bool {
	if s := r.sessState.Load(); s != nil {
		return s.status == GamerStatus_Online
	}
	return false
}
func (r *Gamer) Doc() *configdoc.DocPbConfig        { return r.doc }
func (r *Gamer) DocExt() *configdoc.DocExtendConfig { return r.docExtend }
func (r *Gamer) GetModel() *gamedata.GamerModel     { return r.Model }
func (r *Gamer) Now() int64                         { return xtime.NowUnix() }
func (r *Gamer) NowMs() int64                       { return xtime.NowUnixMs() }
func (r *Gamer) RandInt(min, max int32) int32 {
	if max <= min {
		return min
	}
	return min + r.rand.Int31n(max-min+1)
}
func (r *Gamer) RandFloat() float32 { return r.rand.Float32() }
func (r *Gamer) Logger() iface.ILogger {
	if r.logger != nil {
		return r.logger
	}
	return r
}

func (r *Gamer) Task() iface.ITaskModule         { return r.taskModule }
func (r *Gamer) Item() iface.IItemModule         { return r.itemModule }
func (r *Gamer) Hero() iface.IHeroModule         { return r.heroModule }
func (r *Gamer) Shop() iface.IShopModule         { return r.shopModule }
func (r *Gamer) Mail() iface.IMailModule         { return r.mailModule }
func (r *Gamer) Activity() iface.IActivityModule { return r.activityModule }
func (r *Gamer) Player() iface.IPlayerModule     { return r.playerModule }
func (r *Gamer) Function() iface.IFunctionModule { return r.functionModule }
func (r *Gamer) Match() iface.IMatchModule       { return r.matchModule }

func (r *Gamer) SendMsg(m proto.Message)                      { r.sendWithCode(errorpb.ERROR_SUCCESS, m) }
func (r *Gamer) SendCode(code errorpb.ERROR, m proto.Message) { r.sendWithCode(code, m) }
func (r *Gamer) SendErrorCode(msgId pb.MSG_ID, err errorpb.ERROR) {
	ps, gs := r.GetSessIds()
	server.MS.NetMgr.SendMsg2Sess(gs, msg.NewMsgWithCode(msgId, err, nil).SetUserInfo(ps, r.GamerId), nil)
}
func (r *Gamer) sendWithCode(code errorpb.ERROR, data proto.Message) {
	ps, gs := r.GetSessIds()
	server.MS.NetMgr.SendMsg2Sess(gs, msg.NewRspMsgWithProtoAndCode(0, code, data).SetUserInfo(ps, r.GamerId), nil)
}
func (r *Gamer) Post(f func()) { _ = r.AddMsgTask(0, f) }
func (r *Gamer) OnDocReload(doc *configdoc.DocPbConfig, docExt *configdoc.DocExtendConfig) {
	r.doc, r.docExtend = doc, docExt
}
func (r *Gamer) Init()  {}
func (r *Gamer) Start() { go r.drainMsgChan() }
func (r *Gamer) drainMsgChan() {
	for f := range r.msgChan {
		if f != nil {
			f()
		}
	}
}
func (r *Gamer) IsStop() bool { return r.stopped.Load() }
func (r *Gamer) Stop(reason string) {
	if r.stopped.CompareAndSwap(false, true) {
		xlog.Infof("gamer stop gid:%d reason:%s", r.GamerId, reason)
		close(r.msgChan)
	}
}
func (r *Gamer) tick() {}
func (r *Gamer) AddMsgTask(_ pb.MSG_ID, f func()) error {
	if r.IsStop() {
		return ErrGamerStopped
	}
	select {
	case r.msgChan <- f:
		return nil
	default:
		return ErrGamerTaskQueueFull
	}
}
func (r *Gamer) LoginFirst(now int64) {}
func (r *Gamer) OnLogin(now int64, dayFirstLogin bool, changeDevice bool) {

}
func (r *Gamer) LoginAfter(reconnect bool, changeDevice bool) bool { return true }
func (r *Gamer) Save(stopSave bool) {
	if r.Model != nil {
		_ = r.Model.Save()
	}
}
func (r *Gamer) Offline(reason string, force bool) { r.offline(reason, force, 0, false) }
func (r *Gamer) OfflineIfSessionMatch(playerSessId int64, reason string, force bool) bool {
	return r.offline(reason, force, playerSessId, true)
}
func (r *Gamer) offline(reason string, force bool, playerSessId int64, needMatch bool) bool {
	s := r.sessState.Load()
	if s == nil {
		return false
	}
	if needMatch && s.playerSessId != playerSessId {
		return false
	}
	r.SetOnlineStatus(GamerStatus_Offline)
	xlog.Infof("gamer offline gid:%d reason:%s", r.GamerId, reason)
	return true
}
func (r *Gamer) Online(gateSvrSession, playerSession int64) {
	r.updateSessState(func(s *GamerSessState) {
		s.gateSessId = gateSvrSession
		s.playerSessId = playerSession
		s.status = GamerStatus_Online
	})
}
func (r *Gamer) GetLastPassDayTime() int64 { return 0 }
func (r *Gamer) OnPassDay(now int64)       {}
func (r *Gamer) SaveDocRaw() bson.Raw {
	if r.Model != nil {
		return r.Model.SaveDocRaw()
	}
	return nil
}

func NewGamer(gid int64, gateSvrSession, playerSession int64) (*Gamer, error) {
	cfg := server.MS.ConfBase
	var doc *configdoc.DocPbConfig
	var docExt *configdoc.DocExtendConfig
	if cfg != nil {
		doc, docExt = cfg.Doc, cfg.DocExtend
	}
	model, err := gamedata.NewGamerModel(gid, nil, nil)
	if err != nil {
		model = gamedata.NewGamerModelProxy(gid)
	}
	g := &Gamer{GamerId: gid, Model: model, doc: doc, docExtend: docExt, msgChan: make(chan func(), GAMER_MSG_CHAN_SIZE), rand: rand.New(rand.NewSource(time.Now().UnixNano() + gid))}
	g.bindModules()
	g.Online(gateSvrSession, playerSession)
	return g, nil
}

func (r *Gamer) bindModules() {
	if r == nil || r.Model == nil {
		return
	}
	r.matchModule = matchmodule.NewMatchModule(r, r.Model)
	r.taskModule = taskmodule.NewTaskModule(r, r.Model)
	r.shopModule = shopmodule.NewShopModule(r, r.Model)
	if mod := r.Model.GamerMods[persist.GamerHeroModIndex]; mod != nil {
		if v, ok := mod.(iface.IHeroModule); ok {
			r.heroModule = v
		}
	}
	if mod := r.Model.GamerMods[persist.GamerPackModIndex]; mod != nil {
		if v, ok := mod.(iface.IPackModule); ok {
			r.packModule = v
		}
	}
	if mod := r.Model.GamerMods[persist.GamerFunctionModIndex]; mod != nil {
		if v, ok := mod.(iface.IFunctionModule); ok {
			r.functionModule = v
		}
	}
}
