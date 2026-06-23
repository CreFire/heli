package xtime

import (
	"fmt"
	"testing"
	"time"
)

func TestNow(t *testing.T) {
	loc := time.FixedZone("local", int(LocalZoneOffset))

	// 重置GM时间偏移
	SetGmAdd(0)

	// 测试正常情况
	now1 := Now()
	if now1.IsZero() {
		t.Errorf("Expected non-zero time when GmAdd is 0")
	}

	// 测试GM时间偏移
	SetGmAdd(3600) // 增加1小时
	now2 := Now()
	diff := now2.Unix() - time.Now().In(loc).Unix()
	if diff < 3595 || diff > 3605 {
		t.Errorf("Expected Now() with GM offset close to +3600s, got diff=%d", diff)
	}

	// 恢复
	SetGmAdd(0)
}

func TestNowUnix(t *testing.T) {
	SetGmAdd(0)

	normalTime := time.Now().Unix()
	ourTime := NowUnix()

	// 允许1秒的误差
	if abs(normalTime-ourTime) > 1 {
		t.Errorf("NowUnix() = %d, want close to %d", ourTime, normalTime)
	}

	// 测试GM时间偏移
	SetGmAdd(3600)
	ourTimeWithOffset := NowUnix()
	if ourTimeWithOffset <= ourTime {
		t.Error("NowUnix with GM offset should be greater")
	}

	SetGmAdd(0)
}

func TestNowUnixMs(t *testing.T) {
	SetGmAdd(0)

	normalTime := time.Now().UnixMilli()
	ourTime := NowUnixMs()

	// 允许100毫秒的误差
	if abs(normalTime-ourTime) > 100 {
		t.Errorf("NowUnixMs() = %d, want close to %d", ourTime, normalTime)
	}

	SetGmAdd(3600)
	ourTimeWithOffset := NowUnixMs()
	if ourTimeWithOffset <= ourTime {
		t.Error("NowUnixMs with GM offset should be greater")
	}

	SetGmAdd(0)
}

func TestCheckDailyFresh(t *testing.T) {
	// 测试同一天
	today := time.Now().Unix()
	yesterday := today - DaySec

	if CheckDailyFresh(today, today) {
		t.Error("Same day should not be fresh")
	}

	if !CheckDailyFresh(yesterday, today) {
		t.Error("Different days should be fresh")
	}
}

func TestCheckWeeklyFresh(t *testing.T) {
	now := time.Now().Unix()
	lastWeek := now - WeekSec

	if CheckWeeklyFresh(now, now) {
		t.Error("Same week should not be fresh")
	}

	if !CheckWeeklyFresh(lastWeek, now) {
		t.Error("Different weeks should be fresh")
	}
}

func TestCheckMonthlyFresh(t *testing.T) {
	now := time.Now().Unix()

	// 获取上个月的时间
	lastMonthTime := time.Now().AddDate(0, -1, 0)
	lastMonth := lastMonthTime.Unix()

	if CheckMonthlyFresh(now, now) {
		t.Error("Same month should not be fresh")
	}

	if !CheckMonthlyFresh(lastMonth, now) {
		t.Error("Different months should be fresh")
	}
}

func TestGetLocalDayZeroUnix(t *testing.T) {
	now := time.Now().Unix()
	shifted := now + LocalZoneOffset
	expected := shifted - shifted%DaySec - LocalZoneOffset
	result := GetLocalDayZeroUnix(now)

	if result != expected {
		t.Errorf("GetLocalDayZeroUnix() = %d, want %d", result, expected)
	}
}

func TestGetLocalWeekZeroUnix(t *testing.T) {
	loc := time.FixedZone("local", int(LocalZoneOffset))
	now := time.Now().In(loc)

	// 计算本周一的零点
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // 周日作为一周的最后一天
	}
	daysToMonday := (weekday - 1 + 7) % 7
	monday := now.AddDate(0, 0, -daysToMonday)
	expected := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, loc).Unix()

	result := GetLocalWeekZeroUnix(now.Unix())

	if result != expected {
		t.Errorf("GetLocalWeekZeroUnix() = %d, want %d", result, expected)
	}
}

func TestGetLocalMonthZeroUnix(t *testing.T) {
	loc := time.FixedZone("local", int(LocalZoneOffset))
	now := time.Now().In(loc)
	expected := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).Unix()
	result := GetLocalMonthZeroUnix(now.Unix())

	if result != expected {
		t.Errorf("GetLocalMonthZeroUnix() = %d, want %d", result, expected)
	}
}

func TestTimeToString(t *testing.T) {
	loc := time.FixedZone("local", int(LocalZoneOffset))
	testTime := time.Date(2023, 12, 25, 15, 30, 45, 0, loc)
	expected := "2023-12-25 15:30:45"

	result, err := TimeToString(testTime, time.DateTime)
	if err != nil {
		t.Errorf("TimeToString error: %v", err)
	}

	if result != expected {
		t.Errorf("TimeToString() = %s, want %s", result, expected)
	}
}

func TestTimestampToString(t *testing.T) {
	loc := time.FixedZone("local", int(LocalZoneOffset))
	testTime := time.Date(2023, 12, 25, 15, 30, 45, 0, loc)
	timestamp := testTime.Unix()
	expected := "2023-12-25 15:30:45"

	result, err := TimestampToString(timestamp, time.DateTime)
	if err != nil {
		t.Errorf("TimestampToString error: %v", err)
	}

	if result != expected {
		t.Errorf("TimestampToString() = %s, want %s", result, expected)
	}
}

func TestStringToTime(t *testing.T) {
	timeStr := "2023-12-25 15:30:45"
	loc := time.FixedZone("local", int(LocalZoneOffset))
	expected := time.Date(2023, 12, 25, 15, 30, 45, 0, loc)

	result, err := StringToTime(timeStr, time.DateTime)
	if err != nil {
		t.Errorf("StringToTime error: %v", err)
	}

	// 比较时间戳，避免时区问题
	if result.Unix() != expected.Unix() {
		t.Errorf("StringToTime() = %v, want %v", result, expected)
	}
}

func TestStringToTimeUnix(t *testing.T) {
	timeStr := "2023-12-25 15:30:45"
	loc := time.FixedZone("local", int(LocalZoneOffset))
	expected := time.Date(2023, 12, 25, 15, 30, 45, 0, loc).Unix()

	result, _ := StringToTimeUnix(timeStr, time.DateTime)

	if result != expected {
		t.Errorf("StringToTimeUnix() = %d, want %d", result, expected)
	}
}

// 辅助函数：计算绝对值
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// 测试带偏移的函数
func TestWithOffsetFunctions(t *testing.T) {
	loc := time.FixedZone("local", int(LocalZoneOffset))
	OffSet = 4 * 3600 // 确保与默认配置一致

	// 29日03:00本地，应回滚到28日04:00（默认4点刷新）
	tsEarly := time.Date(2024, 1, 29, 3, 0, 0, 0, loc).Unix()
	expectedEarly := time.Date(2024, 1, 28, 4, 0, 0, 0, loc).Unix()
	if dayZero := GetLocalDayZeroUnixWithOffSet(tsEarly); dayZero != expectedEarly {
		t.Errorf("Daily refresh before offset incorrect, got %d, want %d", dayZero, expectedEarly)
	}

	// 29日05:00本地，应归属29日04:00
	tsLate := time.Date(2024, 1, 29, 5, 0, 0, 0, loc).Unix()
	expectedLate := time.Date(2024, 1, 29, 4, 0, 0, 0, loc).Unix()
	if dayZero := GetLocalDayZeroUnixWithOffSet(tsLate); dayZero != expectedLate {
		t.Errorf("Daily refresh after offset incorrect, got %d, want %d", dayZero, expectedLate)
	}

	// 自定义2小时偏移，29日01:00应回滚到28日02:00
	customOffset := int64(2 * 3600)
	tsCustom := time.Date(2024, 1, 29, 1, 0, 0, 0, loc).Unix()
	expectedCustom := time.Date(2024, 1, 28, 2, 0, 0, 0, loc).Unix()
	if dayZero := GetLocalDayZeroUnixWithCustomOffSet(tsCustom, customOffset); dayZero != expectedCustom {
		t.Errorf("Custom daily refresh incorrect, got %d, want %d", dayZero, expectedCustom)
	}

	// 测试每周刷新时间
	now := time.Now().Unix()
	weekZero := GetLocalWeekZeroUnixWithOffset(now)
	weekZeroCustom := GetLocalWeekZeroUnixWithCustomOffset(now, 7200)

	if weekZeroCustom-weekZero != 7200-OffSet {
		t.Error("Custom offset not working correctly for weekly refresh")
	}
}

func TestGetLocalWeekZeroUnixWithOffset(t *testing.T) {
	d := NextLocalDayZeroSec() % 3600
	fmt.Println(d)
}
