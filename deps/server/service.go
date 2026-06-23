package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"game/deps/async"
	"game/deps/basal"
	"game/deps/etcd"
	"game/deps/eventbus"
	"game/deps/fastid"
	"game/deps/misc"
	"game/deps/mongoclient"
	"game/deps/netmgr"
	"game/deps/netmgr/options"
	redisclient "game/deps/redis"
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	servicemgr "game/deps/service_mgr"
	timermgr "game/deps/timer_mgr"
	"game/deps/xhttp"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/configdoc"
	"game/src/proto/eventpb"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/google/gops/agent"
	"github.com/samber/lo"
	"github.com/spf13/pflag"
)

const (
	USR_RELOAD_SIGNAL  = syscall.Signal(60)
	MEM_PROFILE_SIGNAL = syscall.Signal(59)
	CPU_PROFILE_SIGNAL = syscall.Signal(58)
)

type Server struct {
	GameService
	ConfBase     *configdoc.ConfigBase
	SvrMgr       *servicemgr.Manager
	TimerMgr     *timermgr.TimerMgr
	MongoDB      *mongoclient.MongoClient
	RedisDB      *redisclient.RedisClient
	Etcd         *etcd.EtcdClient
	Async        *async.Async
	TaskQueue    chan func()
	EventQueue   chan *eventpb.Event
	EventBus     eventbus.Bus
	NetMgr       *netmgr.NetMgr
	HttpServe    *xhttp.HttpServer
	BackendServe *xhttp.HttpServer
	Rpc          *rpcmgr.RpcMgr
	Router       *router.Router
	LaunchTime   time.Time
	confPath     string
	quit         chan struct{}
	wg           sync.WaitGroup
	Stopping     bool
}

func (s *Server) Init(service GameService) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic %v", r)
			fmt.Fprintf(os.Stderr, "init server panic, err: %v \n stack: %v", err, string(debug.Stack()))
		}
	}()
	if service == nil {
		fmt.Fprintf(os.Stderr, "service is not initialized \n")
		return fmt.Errorf("service is not initialized")
	}
	s.LaunchTime = xtime.Now()

	confPath, err := s.loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load yaml config failed; err: %s \n", err.Error())
		return err
	}

	// init log based on YAML config before loading doc tables
	xlog.InitDefaultLoggerWithOptions(configdoc.LogCfgToOptions(s.ConfBase.GetSvrCfg().Log))
	xlog.Infof("")
	xlog.Infof("----------server start %s----------", xtime.Now().Format("2006-01-02 15:04:05.000"))
	js, _ := json.Marshal(s.ConfBase.GetSvrCfg())
	xlog.Infof("server conf: %v", string(js))
	js, _ = json.Marshal(s.ConfBase.GetGlobalCfg())
	xlog.Infof("global conf: %v", string(js))
	if err := s.ConfBase.LoadTableConfig(); err != nil {
		xlog.Errorf("load doc cfg failed. gameDataPath:%s err:%v", s.ConfBase.Global.GameDataPath, err)
		return err
	}

	//设置fastid machineid ,conf serverid
	fastid.InitWithMachineID(s.ConfBase.GetSvrCfg().Id)
	//TODO disable deadlock checker

	SetUpDeadlockChecker(s.ConfBase.GetGlobalCfg().EnableLockCheck)

	// init service
	s.GameService = service
	asy, err := async.NewAsync(s.ConfBase.GetSvrCfg().AsyncPoolSize, s.ConfBase.GetSvrCfg().ASyncQueueSize)
	if err != nil {
		xlog.Errorf("init async failed; err: %s \n", err.Error())
		return err
	}

	s.TaskQueue = make(chan func(), 10000)
	s.EventQueue = make(chan *eventpb.Event, 10000)
	s.quit = make(chan struct{})
	s.confPath = confPath
	s.Async = asy
	s.EventBus = eventbus.NewMemoryBus(s.Async, func(evt *eventpb.Event) uint64 {
		return uint64(evt.GetGamerId())
	})
	s.NetMgr = netmgr.NewNetMgr()
	s.NetMgr.SetLocalServer(s.ConfBase.GetSvrCfg().Type, s.ConfBase.GetSvrCfg().Id)
	s.TimerMgr = timermgr.NewTimerMgr()
	s.HttpServe = xhttp.NewHttpServer()
	s.Rpc = rpcmgr.NewRpcMgr(s.NetMgr)
	s.Router = router.NewRouter(s.Rpc)
	s.BackendServe = xhttp.NewHttpServer()
	err = s.GameService.OnInit()
	if err != nil {
		return err
	}
	xlog.Infof("server init ok")
	return nil
}

func (s *Server) loadConfig() (string, error) {
	//解析配置路径
	pflag.String("f", "", "--f='./config.json'")
	pflag.Parse()
	confPath := pflag.Lookup("f").Value.String()
	if confPath == "" {
		xlog.Errorf("config file path is empty")
		return "", fmt.Errorf("config file path is empty")
	}

	// 加载服务器配置
	s.ConfBase = &configdoc.ConfigBase{}
	err := s.ConfBase.LoadYamlConfig(confPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config file failed, confPath: %s, err: %v \n", confPath, err)
		return "", err
	}
	return confPath, nil
}

func (s *Server) Start() (err error) {
	defer func() {
		if r := recover(); r != nil {
			xlog.Errorf("init server panic, err: %v \n stack: %v", err, string(debug.Stack()))
			err = fmt.Errorf("panic %v", r)
		}
	}()
	xlog.Infof("server start...")
	// gops agent
	gopsAgentAddr := s.ConfBase.GetSvrCfg().GopsAddr
	gopsAgentAddr = lo.Ternary(gopsAgentAddr == "", ":0", gopsAgentAddr)
	if err := agent.Listen(agent.Options{Addr: gopsAgentAddr}); err != nil {
		xlog.Errorf("gops agent.Listen failed error: %v", err)
		return err
	}
	//backend
	backendAddr := s.ConfBase.GetSvrCfg().BackendAddr
	backendAddr = lo.Ternary(backendAddr == "", ":0", backendAddr)
	if err := s.BackendServe.StartServe(backendAddr); err != nil {
		xlog.Errorf("backendServe.Init error: %v", err)
		return err
	}

	err = s.GameService.BeforeStart()
	if err != nil {
		xlog.Errorf("service BeforeStart error: %v", err)
		return err
	}

	skipEtcd := s.ConfBase.GetSvrCfg().Type == "robot" || s.ConfBase.GetSvrCfg().NotEtcd
	if skipEtcd {
		xlog.Infof("skip etcd for service type=%s notEtcd=%v", s.ConfBase.GetSvrCfg().Type, s.ConfBase.GetSvrCfg().NotEtcd)
	} else {
		s.Etcd, err = etcd.NewEtcdClient(s.ConfBase.GetGlobalCfg().EtcdDsn, xlog.DefaultLogger)
		if err != nil {
			xlog.Warnf("etcd:%v", s.ConfBase.GetGlobalCfg().EtcdDsn)
			xlog.Errorf("NewEtcdClient error: %v", err)
			return err
		}
		xlog.Infof("etcd  connect ok! etcdDsn: %s", s.ConfBase.GetGlobalCfg().EtcdDsn)
	}

	s.MongoDB, err = mongoclient.NewMongoClient(s.ConfBase.GetGlobalCfg().Mongo.Dsn, xlog.DefaultLogger)
	if err != nil {
		xlog.Errorf("NewMongoClient error: %v", err)
		return err
	}
	xlog.Infof("mongo connect ok! mongodbDsn: %s  dataBase: %s ", s.ConfBase.GetGlobalCfg().Mongo.Dsn, s.ConfBase.GetGlobalCfg().Mongo.DbName)

	s.RedisDB, err = redisclient.NewRedisClient(s.ConfBase.GetGlobalCfg().RedisDsn, xlog.DefaultLogger)
	if err != nil {
		xlog.Errorf("NewRedisClient error: %v", err)
		return err
	}
	xlog.Infof("redis connect ok! redisDsn: %s", s.ConfBase.GetGlobalCfg().RedisDsn)

	s.TimerMgr.Start()
	s.Async.Start()

	s.NetMgr.RegisterTimerMgr(s.TimerMgr)
	if !skipEtcd {
		s.NetMgr.RegisterCanReconnect(func(params *options.ConnectParams) bool {
			if s.SvrMgr == nil {
				return false
			}
			ins := s.SvrMgr.List(params.SvrType, func(instance *servicemgr.ServiceInstance) bool {
				return params.SvrId == instance.InstanceId &&
					params.ConnectAddr == fmt.Sprintf("%v:%v", instance.Host, instance.Port) &&
					instance.Enable &&
					instance.Healthy == servicemgr.ServiceStatusHealth
			})
			return len(ins) != 0
		})
	}
	s.NetMgr.Start()

	if !skipEtcd {
		s.SvrMgr, err = servicemgr.NewUnregistered(s.ConfBase.GetSvrCfg(), s.Etcd)
		if err != nil {
			xlog.Errorf("servicemgr.NewUnregistered error: %v", err)
			return err
		}
	}

	if err := s.GameService.OnStart(); err != nil {
		xlog.Errorf("service OnStart error: %v", err)
		return err
	}

	if s.SvrMgr != nil {
		if err := s.SvrMgr.Register(); err != nil {
			xlog.Errorf("service register error: %v", err)
			return err
		}
	}

	s.Rpc.Start()
	xlog.Infof("server start ok!")
	go s.run()

	return nil
}

func (s *Server) run() {
	t := time.NewTicker(time.Second)
	s.wg.Add(1)

	defer t.Stop()
	defer s.wg.Done()
	for {
		select {
		case <-s.quit:
			s.drainTaskAndEvent()
			return
		case f := <-s.TaskQueue:
			basal.SafeRun(f)
		case evt := <-s.EventQueue:
			basal.SafeRun(func() {
				s.GameService.OnEventHandle(evt)
			})
		case now := <-t.C:
			basal.SafeRun(func() {
				nowUnix := xtime.TimeToUnix(now)
				if err := s.GameService.OnHeart(nowUnix); err != nil {
					xlog.Errorf("service Heart error: %v", err)
				}
			})
		}
	}
}

func (s *Server) drainTaskAndEvent() {
taskLoop:
	for {
		select {
		case f := <-s.TaskQueue:
			basal.SafeRun(f)
		default:
			break taskLoop
		}
	}

eventLoop:
	for {
		select {
		case evt := <-s.EventQueue:
			basal.SafeRun(func() {
				s.GameService.OnEventHandle(evt)
			})
		default:
			break eventLoop
		}
	}
}

func (s *Server) WaitStop() {

	notify := make(chan os.Signal, 1)
	signal.Notify(notify, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT, syscall.SIGPIPE, USR_RELOAD_SIGNAL, MEM_PROFILE_SIGNAL, CPU_PROFILE_SIGNAL)
	for {
		signal := <-notify
		xlog.Infof("listen signal...%v", signal.String())
		switch signal {
		case syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT:
			xlog.Infof("catch signal %s : quit", signal.String())
			s.stop()
			return
		case USR_RELOAD_SIGNAL:
			xlog.Infof("catch signal %s : reload", signal.String())
			s.Reload()
		case MEM_PROFILE_SIGNAL:
			basal.WriteMemProfile()
		case CPU_PROFILE_SIGNAL:
			basal.WriteCPUProfile()
		}
	}
}

func (s *Server) Shutdown(exitCode int) {
	if s == nil {
		os.Exit(exitCode)
		return
	}
	go func() {
		if err := s.stop(); err != nil && exitCode == 0 {
			exitCode = 1
		}
		os.Exit(exitCode)
	}()
}

func (s *Server) stop() (retErr error) {
	s.Stopping = true

	defer func() {
		if r := recover(); r != nil {
			xlog.Errorf("init server panic, err: %v", r)
			retErr = errors.Join(retErr, fmt.Errorf("panic %v", r))
		}
	}()

	if s.SvrMgr != nil {
		if err := s.SvrMgr.UpdateSelf(func(inst *servicemgr.ServiceInstance) {
			inst.Healthy = servicemgr.ServiceStatusStopping
		}); err != nil {
			xlog.Warnf("set service health to stopping failed: %v", err)
		}
	}

	if s.HttpServe != nil && s.HttpServe.HttpServer != nil {
		if err := s.HttpServe.StopServe(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("HttpServe.StopServe: %w", err))
		}
	}

	if s.BackendServe != nil && s.BackendServe.HttpServer != nil {
		if err := s.BackendServe.StopServe(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("BackendServe.StopServe: %w", err))
		}
	}

	var beforeStopErr error
	if err := basal.SafeRun(func() {
		beforeStopErr = s.GameService.BeforeStop()
	}); err != nil {
		retErr = errors.Join(retErr, fmt.Errorf("BeforeStop panic: %w", err))
	} else if beforeStopErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("BeforeStop: %w", beforeStopErr))
	}

	if err := s.TimerMgr.Stop(); err != nil {
		retErr = errors.Join(retErr, fmt.Errorf("TimerMgr.Stop: %w", err))
	}

	time.Sleep(10 * time.Millisecond)

	s.NetMgr.StopFront()
	agent.Close()

	var onStopErr error
	if err := basal.SafeRun(func() {
		onStopErr = s.GameService.OnStop()
	}); err != nil {
		retErr = errors.Join(retErr, fmt.Errorf("OnStop panic: %w", err))
	} else if onStopErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("OnStop: %w", onStopErr))
	}

	if s.EventBus != nil {
		s.EventBus.Close()
	}

	s.Async.Stop()

	close(s.quit)
	s.wg.Wait()

	s.Rpc.Stop()
	s.NetMgr.Stop()

	if s.SvrMgr != nil {
		s.SvrMgr.Close()
	}

	if s.Etcd != nil {
		s.Etcd.Close()
	}
	if s.MongoDB != nil {
		s.MongoDB.Close()
	}
	if s.RedisDB != nil {
		s.RedisDB.Close()
	}

	if retErr != nil {
		xlog.Warnf("server stop finished with errors: %v", retErr)
	} else {
		xlog.Infof("server stop success")
	}
	xlog.Close()
	return retErr
}

func (s *Server) Reload() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic %v", r)
			xlog.Errorf("reload panic, err: %v \n stack: %v", err, string(debug.Stack()))
		}
	}()

	xlog.Infof("[%v-%v] reload config start", s.ConfBase.GetSvrCfg().Type, s.ConfBase.GetSvrCfg().Id)
	newCfg, err := s.ConfBase.Reload(s.confPath)
	if err != nil {
		xlog.Warnf("[%v-%v] reload config failed. err:%v", s.ConfBase.GetSvrCfg().Type, s.ConfBase.GetSvrCfg().Id, err)
		return err
	}
	oldCfg := s.ConfBase
	if err := s.GameService.OnReload(oldCfg, newCfg); err != nil {
		xlog.Warnf("[%v-%v] reload config failed. err:%v", s.ConfBase.GetSvrCfg().Type, s.ConfBase.GetSvrCfg().Id, err)
		return err
	}

	if err := s.SvrMgr.UpdateSelf(func(inst *servicemgr.ServiceInstance) {
		inst.ConfVersion = int32(newCfg.Doc.ExcelVersion)
	}); err != nil {
		xlog.Errorf("[%v-%v] reload sync service instance version failed. confVersion:%d err:%v",
			s.ConfBase.GetSvrCfg().Type, s.ConfBase.GetSvrCfg().Id, newCfg.Doc.ExcelVersion, err)
		return err
	}

	misc.ExcelVer = strconv.FormatInt(int64(newCfg.Doc.ExcelVersion), 10)
	s.ConfBase = newCfg

	xlog.Infof("[%v-%v] reload config finished. ", s.ConfBase.GetSvrCfg().Type, s.ConfBase.GetSvrCfg().Id)
	if logCfg := s.ConfBase.GetSvrCfg().Log; logCfg != nil {
		xlog.SetLogLevel(logCfg.Level)
	}
	js, _ := json.Marshal(s.ConfBase.GetSvrCfg())
	xlog.Infof("server reload conf: %v", string(js))
	return nil
}

func (s *Server) PostMainTask(f func()) error {
	select {
	case s.TaskQueue <- f:
	default:
		return fmt.Errorf("task queue is full")
	}
	return nil
}

func (s *Server) PostAsyncTask(key uint64, tag string, f func()) error {
	if key == 0 {
		return s.Async.Post(tag, f)
	} else {
		return s.Async.PostFixed(key, tag, f)
	}
}

func (s *Server) PostEvent(evt *eventpb.Event) error {
	if evt == nil {
		return nil
	}
	select {
	case s.EventQueue <- evt:
	default:
		return fmt.Errorf("event queue is full")
	}
	return nil
}

var MS = &Server{}
