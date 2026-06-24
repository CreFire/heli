package common

import "google.golang.org/protobuf/proto"

const InnerServerTypeRobot = "robot"
const InnerServerTypeAuth = "auth"
const InnerServerTypeLogic = "logic"
const InnerServerTypeBattle = "battle"
const InnerServerTypeGate = "gate"
const (
	ServerType_Robot = iota
	ServerType_Auth
	ServerType_Logic
	ServerType_Gate
	ServerType_Battle
)

var ServerTypeMap = map[string]int32{
	InnerServerTypeRobot:  ServerType_Robot,
	InnerServerTypeAuth:   ServerType_Auth,
	InnerServerTypeLogic:  ServerType_Logic,
	InnerServerTypeBattle: ServerType_Battle,
	InnerServerTypeGate:   ServerType_Gate,
}

var ServerTypeNameMap = map[int32]string{
	ServerType_Robot:  InnerServerTypeRobot,
	ServerType_Auth:   InnerServerTypeAuth,
	ServerType_Logic:  InnerServerTypeLogic,
	ServerType_Battle: InnerServerTypeBattle,
	ServerType_Gate:   InnerServerTypeGate,
}

const (
	ServerOffset = 1000
)

func GetServerType(serverName string) int32 {
	return ServerTypeMap[serverName]
}

const (
	GID_MIN = 1024
)

const (
	FUNC_GAMER_UPDATE_HANDLER_SLOW_MS = 300 //gamer update handler函数耗时阈值
	FUNC_LOGIN_CHECK_SLOW_MS2         = 500 //login check函数耗时阈值
)

const (
	DEFAULT_UNIQUE_FOREVER_UNLOCK       = -1    // 永久解锁
	DEFAULT_WEIGHT_LEN            int32 = 10000 // 默认权重大小
	DEFAULT_QUEUE_LEN                   = 5     // 默认队列长度
)

const ( // 重置任务
	TASK_RESET_NONE    = 0 // 不重置
	TASK_RESET_DAILY   = 1 // 每日重置
	TASK_RESET_WEEKLY  = 2 // 每周重置
	TASK_RESET_MONTHLY = 3 // 每月重置
)

type Message proto.Message

const (
	SHOP_REFRESH_NO       int32 = 0 //不刷新
	SHOP_REFRESH_DAY      int32 = 1 //日刷新
	SHOP_REFRESH_WEEK     int32 = 2 //周刷新
	SHOP_REFRESH_MONTH    int32 = 3 //月刷新
	SHOP_REFRESH_BUY_TIME int32 = 4 //按购买时间之后x天刷新
)

const (
	SHOP_TYPE_FIXED  int32 = 1 //固定商店
	SHOP_TYPE_RANDOM int32 = 2 //随机商店
)

const (
	PASS_FREE     = 0 //  免费
	PASS_UNLOCKED = 1 //  解锁
)

const (
	GAME_AREA_PROD   = "prod"
	GAME_AREA_DEV    = "dev"
	GAME_AREA_QA     = "qa"
	GAME_AREA_STRESS = "stress"
)
