package common

import (
	"math"
	"time"
)

const GAMER_DATA_CACHE_TIME = 60 * 5        //玩家数据缓存时间 3分钟
const GAMER_HEART_TIME_OUT = 30             //玩家心跳超时时间 30秒
const GAMER_HEART_SEND_INTERVAL = 10        //玩家心跳发送间隔 10秒
const GAMER_LOGIC_SAVE_TIME = 60 * 4        //玩家逻辑层保存时间 60秒
const GATE_CACHE_USER_TIME = 60             //GATE 缓存玩家数据时间 60秒
const GAMER_MAX_MAIL_NUM = 100              //玩家最大邮件数量
const MAIL_MAX_LIFE_TIME = 60 * 60 * 24 * 7 //邮件最大存活时间 30天

// 高8位存储难度(int8)，低56位存储伤害(int64)
func MergeInt8AndInt64(high int8, low int64) int64 {
	return (int64(high) << 56) | (low & 0x00FFFFFFFFFFFFFF)
}

// SplitInt64ToInt8AndInt64 将合并后的 int64 拆分为 int8 和 int64
func SplitInt64ToInt8AndInt64(merged int64) (high int8, low int64) {
	// 1. 反算 high：提取最高 8 位，右移 56 位后转 int8
	high = int8(merged >> 56)

	// 2. 反算 low：提取低 56 位（与原合并时的掩码一致）
	low = merged & 0x00FFFFFFFFFFFFFF

	return high, low
}

// WeekdayTo1_7 将 time.Weekday 转换为 1-7 的整数（周日为 7）
func WeekdayTo1_7(w time.Weekday) int32 {
	switch w {
	case time.Sunday:
		return 7
	default:
		return int32(w) // 周一~周六：1~6 直接返回
	}
}

func SafeInt64ToInt32(i64 int64) (int32, bool) {
	if i64 < math.MinInt32 || i64 > math.MaxInt32 {
		return 0, false
	}
	return int32(i64), true
}

func SafeAdd(a, b int32) int32 {
	sum := a + b
	if a > 0 && b > 0 && sum < 0 {
		return math.MaxInt32
	}
	if a < 0 && b < 0 && sum > 0 {
		return math.MinInt32
	}
	return sum
}
