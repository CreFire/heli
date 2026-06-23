package configdoc

var (
	SHOP_TYPE_FIXED  int32 = 0 // 固定商店
	SHOP_TYPE_ONCE         = 1 // 一次性商店
	SHOP_TYPE_EVEDAY       = 2 // 随机商店
)

//---------------------------------任务配置----------------------------------

type TASK_PROGRESS_TYPE int8

// 任务进度类型
const (
	TASK_PROGRESS_ADD           TASK_PROGRESS_TYPE = 0 //累加
	TASK_PROGRESS_CALC          TASK_PROGRESS_TYPE = 1 //直接统计
	TASK_PROGRESS_CALC_COVER    TASK_PROGRESS_TYPE = 2 //直接覆盖(直接覆盖,也要实现直接统计函数,解锁时调用直接统计函数计算进度)
	TASK_PROGRESS_CLEAR         TASK_PROGRESS_TYPE = 3 //满足条件就 清除进度为0
	TASK_PROGRESS_CLEAR_ADD     TASK_PROGRESS_TYPE = 4 //满足条件就 清除进度为0后再加上本次传的值
	TASK_PROGRESS_COVER_GREATER TASK_PROGRESS_TYPE = 5 //传入值大于进度值,可覆盖

)

type TaskConf struct {
	id int32
}

type TASK_TRIGGER_TYPE int32

// 任务触发类型
var (
	TASK_TRIGGER_BATTLE_COUNT      TASK_TRIGGER_TYPE = 5001 // 战斗X次
	TASK_TRIGGER_BATTLE_WIN        TASK_TRIGGER_TYPE = 5002 // 战斗胜利X次
	TASK_TRIGGER_KEEP_WIN          TASK_TRIGGER_TYPE = 5003 // 连续战斗胜利X次
	TASK_TRIGGER_LOGIN             TASK_TRIGGER_TYPE = 5004 // 登录X天
	TASK_TRIGGER_LOGIN_KEEP        TASK_TRIGGER_TYPE = 5005 // 连续登录X天
	TASK_TRIGGER_SIGN              TASK_TRIGGER_TYPE = 5006 // 签到X次
	TASK_PASS_STAGE                TASK_TRIGGER_TYPE = 5007 // 通关X关
	TASK_PASS_AFK_STAGE            TASK_TRIGGER_TYPE = 5008 // 通关挂机x关卡 condition stageId
	TASK_TRIGGER_TASK_FINISH_NUM   TASK_TRIGGER_TYPE = 5009 // 累计完成X类型任务Y个
	TASK_TASK_FINISH               TASK_TRIGGER_TYPE = 5010 // x任务完成Y次
	TASK_TRIGGER_HERO_UP           TASK_TRIGGER_TYPE = 5021 // 进行x次英雄升级
	TASK_TRIGGER_HERO_TIER_UP      TASK_TRIGGER_TYPE = 5022 //进行x次英雄升升阶
	TASK_TRIGGER_BATTLE_TYPE_COUNT TASK_TRIGGER_TYPE = 5101 // 战斗x 类型Y次
	TASK_TRIGGER_BATTLE_TYPE_WIN   TASK_TRIGGER_TYPE = 5102 // 战斗x 类型赢Y次

	TASK_TRIGGER_PLAYER_LV           TASK_TRIGGER_TYPE = 6001  // 玩家等级
	TASK_TRIGGER_HERO_LV             TASK_TRIGGER_TYPE = 6002  // 拥有X等级角色Y个
	TASK_TRIGGER_ADDITEM             TASK_TRIGGER_TYPE = 6003  //累计获得XX道具XX个,
	TASK_TRIGGER_PHASE               TASK_TRIGGER_TYPE = 99901 //阶段任务,完成条件中的任务即可完成阶段任务
	TASK_TRIGGER_PHASE_PROGRESS      TASK_TRIGGER_TYPE = 99902 //阶段任务,完成条件中的任务即可完成阶段任务，显示具体的进度，目标值是配置任务的个数
	TASK_TRIGGER_PHASE_TYPE_PROGRESS TASK_TRIGGER_TYPE = 99903 //阶段任务,完成条件中的任务即可完成阶段任务，显示具体的进度，目标值是配置类型任务的完成个数
)

type TASK_COND_VALUES_TYPE int8

const (
	TASK_COND_VALUES_AND TASK_COND_VALUES_TYPE = 0 //并且
	TASK_COND_VALUES_OR  TASK_COND_VALUES_TYPE = 1 //或者
)

type TASK_CONDITION_TYPE int8

const (
	//任务条件判断类型
	TASK_CONDITION_EQUAL          TASK_CONDITION_TYPE = 1  //等于(0无实际意义，代表任意)
	TASK_CONDITION_EQUAL_NOT_ZREO TASK_CONDITION_TYPE = 2  //等于(0具有实际意义，直接判断)
	TASK_CONDITION_GREATER        TASK_CONDITION_TYPE = 3  //大于等于
	TASK_CONDITION_IN_NOT_ZREO    TASK_CONDITION_TYPE = 4  //存在配置中(0具有实际意义，直接判断)
	TASK_CONDITION_IGNORE         TASK_CONDITION_TYPE = 9  //直接忽略
	TASK_CONDITION_LESSER         TASK_CONDITION_TYPE = 10 //小于等于
	TASK_CONDITION_IN             TASK_CONDITION_TYPE = 11 //存在配置中(0无实际意义，代表任意)

	// 任务重置类型
	DROP_RESET_NONE  = 1 // 一次性购买
	DROP_RESET_DAY   = 2 //每日重置商城
	DROP_RESET_WEEK  = 3 //每周重置商城
	DROP_RESET_MONTH = 4 //每月重置商城
)

const (
	GOODS_BUY_LIMIT_ONOE  = 0 // 无限购
	GOODS_BUY_LIMIT_LEVEL = 1 // 账号等级限购
	GOODS_BUY_LIMIT_STAGE = 2 // 通关制定关卡
	GOODS_BUY_LIMIT_NUM   = 3 // 次数限购
)
