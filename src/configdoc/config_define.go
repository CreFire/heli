package configdoc

// 平台类型
type PLATFORM_TYPE int32

var platforms = []string{"none", "iOS", "Android", "PC", "robot"}

func (m PLATFORM_TYPE) Int32() int32 {
	return int32(m)
}

func (m PLATFORM_TYPE) String() string {
	return platforms[m]
}

const (
	// 平台类型
	PLATFORM_IOS     PLATFORM_TYPE = 1 //ios
	PLATFORM_ANDROID PLATFORM_TYPE = 2 //android
	PLATFORM_PC      PLATFORM_TYPE = 3 //pc
	PLATFORM_ROBOT   PLATFORM_TYPE = 4 //机器人
)
const (
	SERVER_TYPE_AUTH   = 1
	SERVER_TYPE_LOGIC  = 2
	SERVER_TYPE_MASTER = 3
)
const (
	RobotTotalNumber = 5       // 机器人账号倍数
	RobotCurNumber   = 100     // 当前机器人数量
	InterValNumber   = 60 * 60 // 运行间隔
)

const MARSHAL_INT_MAX int32 = 255
const UNMARSHAL_INT_MAX int32 = 65535

const (
	COIN_FASHION = 1 // 时装货币
	COIN_DIAMOND = 2 // 钻石
	COIN_EXP     = 3 // 经验
	COIN_GOLD    = 4 // 金币
)

const (
	GM_ADD_ITEM          = "additem"     //gm 添加道具
	GM_ADD_HERO          = "addhero"     // 添加英雄
	GM_HERO_LEVEL        = "herolv"      // 设置英雄等级
	GM_CHANGE_TASK       = "task"        //gm 测试任务
	GM_CHANGE_STATS      = "stats"       //gm 修改统计数据
	GM_CLEAR_DATA        = "clear"       //gm 清除玩家数据
	GM_CHANGE_MATCH      = "match"       // gm 结算
	GM_CHANGE_ADD_EQUIP  = "equip"       // gm 添加时装
	GM_KICK_GAMER        = "kick"        // gm 踢下线
	GM_CHANGE_ADD_SKIN   = "skin"        // gm 添加武器
	GM_SET_TIME          = "settime"     // gm 设置时间
	GM_DEL_ROLE          = "delrole"     // gm 删除账号角色
	GM_PASS_LEVEL        = "pass"        // gm 通关关卡
	GM_AFK_PASS_LEVEL    = "afk"         // gm 跳关挂机关卡
	GM_TRIGGER_TASK      = "trigger"     // gm 触发任务
	GM_ADD_MAIL          = "addmail"     // gm 发送邮件
	GM_ADD_ALL           = "additemall"  // gm 添加所有道具
	GM_REALM_HP          = "realmhp"     // gm 修改玩家当前血量百分比
	GM_OPEN_ALL_FUNCTION = "openallfunc" // gm 开启所有功能
	GM_LINK_UP           = "linkup"      // gm 链接完成任务
	GM_REALM_SCORE       = "realmscore"  // gm 修改玩家无序秘境积分
	GM_REALM_PASS        = "realmpass"   // gm 修改无序秘境通过
	GM_REALM_DIFF        = "realmdiff"   // gm 调整无序秘境难度
	GM_DEL_ITEM          = "delitem"     // gm 删除道具
	GM_ADD_GUIDES        = "addguides"   // gm 添加引导
	GM_ROBOT_BATTLE_WIN  = "robotbattlewin"
	GM_ROBOT_BATTLE_LOSE = "robotbattlelose"
)

// 刷新时间点
const (
	HOUR4        = 4            //6点刷新
	HOUR4_OFFSET = 3600 * HOUR4 //6点刷新
	HOUR1_SEC    = 3600         //一小时
	DAY1_SEC     = 24 * 3600    //一天
)

const (
	RECORD_TYPE_KEEP_WIN int32 = 1 // 战斗连胜记录
)
