package xtime

import (
	"fmt"
	"game/deps/basal"
	"strings"
	"time"
)

const (
	MinSec         = 60
	HourSec        = MinSec * 60
	DaySec         = HourSec * 24
	WeekSec        = DaySec * 7
	datetimeFormat = "2006-01-02 15:04:05"
	dateFormat     = "2006-01-02"
	timeFormat     = "15:04:05"
)

var (
	GmAdd           = basal.NewNoCacheLineData(int64(0))
	OffSet          int64             // daily refresh offset seconds (default 4*3600)
	LocalZoneOffset = int64(8 * 3600) // fixed zone offset seconds, Beijing default
	loc             *time.Location
)

func init() {
	OffSet = 4 * 3600
	loc = fixedLocal()
}

func fixedLocal() *time.Location {
	return time.FixedZone("local", int(LocalZoneOffset))
}

func SetLocalZoneOffset(zoneOffset int64) {
	LocalZoneOffset = zoneOffset
	loc = fixedLocal()
}

// ---------- zone boundary helpers ----------

// getDayZeroUnixWithZone returns the UTC timestamp for local midnight of a given zone offset (seconds east of UTC).
func getDayZeroUnixWithZone(ts, zoneOffset int64) int64 {
	shifted := ts + zoneOffset
	rem := shifted % DaySec
	if rem < 0 {
		rem += DaySec
	}

	return shifted - rem - zoneOffset
}

// GetDayZeroUnixWithZoneAndOffset returns the UTC timestamp for a daily reset point with a custom offset in the given zone.
func GetDayZeroUnixWithZoneAndOffset(ts, zoneOffset, offset int64) int64 {
	return getDayZeroUnixWithZone(ts-offset, zoneOffset) + offset
}

// getWeekZeroUnixWithZoneAndOffset returns the UTC timestamp for the week start (Monday 00:00) under a zone and daily offset.
func getWeekZeroUnixWithZoneAndOffset(ts, zoneOffset, offset int64) int64 {
	dayZero := GetDayZeroUnixWithZoneAndOffset(ts, zoneOffset, offset)
	shift := ts + zoneOffset - offset
	weekday := time.Unix(shift, 0).UTC().Weekday()
	nd := (int(weekday) - 1 + 7) % 7
	return dayZero - int64(nd)*DaySec
}

// getMonthZeroUnixWithZone returns the UTC timestamp for month start (1st 00:00) under a zone.
func getMonthZeroUnixWithZone(ts, zoneOffset int64) int64 {
	t := time.Unix(ts+zoneOffset, 0).UTC()
	y, m, _ := t.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return first.Unix() - zoneOffset
}

// getMonthZeroUnixWithZoneAndOffset returns the UTC timestamp for month start considering daily offset in a zone.
func getMonthZeroUnixWithZoneAndOffset(ts, zoneOffset, offset int64) int64 {
	return getMonthZeroUnixWithZone(ts-offset, zoneOffset) + offset
}

// ---------- time with GM offset ----------

// Now returns current time with GM offset in the fixed local zone.
func Now() time.Time {
	nowTime := time.Now().In(loc)

	if GmAdd.Get() == 0 {
		return nowTime
	}
	return nowTime.Add(time.Duration(GmAdd.Get()) * time.Second)
}

// NowUnix returns current unix seconds with GM offset.
func NowUnix() int64 {
	return Now().Unix()
}

// NowUnixMs returns current unix milliseconds with GM offset.
func NowUnixMs() int64 {
	return Now().UnixMilli()
}

// TimeToUnix returns unix seconds of a time with GM offset.
func TimeToUnix(now time.Time) int64 {
	return now.Unix()
}

// SetGmAdd sets GM offset seconds.
func SetGmAdd(add int64) {
	GmAdd.UpdateInt(add)
}

func WeekDay() time.Weekday {
	return time.Unix(NowUnix()-OffSet, 0).Weekday()
}

func WeekDayByUnix(ts int64) time.Weekday {
	return time.Unix(ts-OffSet, 0).Weekday()
}

// ---------- zero time getters ----------

// GetLocalDayZeroUnix gets midnight (00:00:00) unix timestamp for a day in the fixed zone.
func GetLocalDayZeroUnix(ts int64) int64 {
	return getDayZeroUnixWithZone(ts, LocalZoneOffset)
}

// GetLocalWeekZeroUnix gets week start (Monday 00:00:00) unix timestamp in the fixed zone.
func GetLocalWeekZeroUnix(ts int64) int64 {
	return getWeekZeroUnixWithZoneAndOffset(ts, LocalZoneOffset, 0)
}

// GetLocalMonthZeroUnix gets month start (1st 00:00:00) unix timestamp in the fixed zone.
func GetLocalMonthZeroUnix(ts int64) int64 {
	return getMonthZeroUnixWithZone(ts, LocalZoneOffset)
}

// ---------- refresh points ----------

// GetLocalDayZeroUnixWithOffSet daily refresh time (default offset)
func GetLocalDayZeroUnixWithOffSet(ts int64) int64 {
	return GetDayZeroUnixWithZoneAndOffset(ts, LocalZoneOffset, OffSet)
}
func NextLocalDayZeroSec() int64 {
	now := NowUnix()
	return DaySec - (now - GetDayZeroUnixWithZoneAndOffset(now, LocalZoneOffset, OffSet))
}

// GetLocalDayZeroUnixWithCustomOffSet daily refresh time (custom offset)
func GetLocalDayZeroUnixWithCustomOffSet(ts int64, offset int64) int64 {
	return GetDayZeroUnixWithZoneAndOffset(ts, LocalZoneOffset, offset)
}

// GetLocalWeekZeroUnixWithOffset weekly refresh time (default offset)
func GetLocalWeekZeroUnixWithOffset(ts int64) int64 {
	return getWeekZeroUnixWithZoneAndOffset(ts, LocalZoneOffset, OffSet)
}

// GetLocalWeekZeroUnixWithCustomOffset weekly refresh time (custom offset)
func GetLocalWeekZeroUnixWithCustomOffset(ts int64, offset int64) int64 {
	return getWeekZeroUnixWithZoneAndOffset(ts, LocalZoneOffset, offset)
}

// ---------- freshness checks ----------

// CheckDailyFresh determines if two timestamps cross the daily boundary (default offset). return true if they cross the boundary.
func CheckDailyFresh(lastSec, nowSec int64) bool {
	return GetDayZeroUnixWithZoneAndOffset(nowSec, LocalZoneOffset, OffSet) !=
		GetDayZeroUnixWithZoneAndOffset(lastSec, LocalZoneOffset, OffSet)
}

// CheckDailyFreshWithOffset determines if two timestamps cross the daily boundary (custom offset).
func CheckDailyFreshWithOffset(lastSec, nowSec int64, offset int64) bool {
	return GetDayZeroUnixWithZoneAndOffset(nowSec, LocalZoneOffset, offset) !=
		GetDayZeroUnixWithZoneAndOffset(lastSec, LocalZoneOffset, offset)
}

// CheckWeeklyFresh determines if two timestamps cross the weekly boundary (default offset).
func CheckWeeklyFresh(lastSec, nowSec int64) bool {
	return getWeekZeroUnixWithZoneAndOffset(nowSec, LocalZoneOffset, OffSet) !=
		getWeekZeroUnixWithZoneAndOffset(lastSec, LocalZoneOffset, OffSet)
}

// CheckWeeklyFreshWithOffset determines if two timestamps cross the weekly boundary (custom offset).
func CheckWeeklyFreshWithOffset(lastSec, nowSec int64, offset int64) bool {
	return getWeekZeroUnixWithZoneAndOffset(nowSec, LocalZoneOffset, offset) !=
		getWeekZeroUnixWithZoneAndOffset(lastSec, LocalZoneOffset, offset)
}

// CheckMonthlyFresh determines if two timestamps cross the monthly boundary (default offset).
func CheckMonthlyFresh(lastSec, nowSec int64) bool {
	return getMonthZeroUnixWithZoneAndOffset(nowSec, LocalZoneOffset, OffSet) !=
		getMonthZeroUnixWithZoneAndOffset(lastSec, LocalZoneOffset, OffSet)
}

// CheckMonthlyFreshWithOffset determines if two timestamps cross the monthly boundary (custom offset).
func CheckMonthlyFreshWithOffset(lastSec, nowSec int64, offset int64) bool {
	return getMonthZeroUnixWithZoneAndOffset(nowSec, LocalZoneOffset, offset) !=
		getMonthZeroUnixWithZoneAndOffset(lastSec, LocalZoneOffset, offset)
}

// ---------- comparisons ----------

// GetDaysBetween returns the day difference between two timestamps (no daily offset).
func GetDaysBetween(ts1, ts2 int64) int {
	days1 := getDayZeroUnixWithZone(ts1, LocalZoneOffset) / DaySec
	days2 := getDayZeroUnixWithZone(ts2, LocalZoneOffset) / DaySec
	return int(days2 - days1)
}

// GetDaysBetweenWithOffset returns the day difference between two timestamps (with daily offset).
func GetDaysBetweenWithOffset(ts1, ts2 int64) int {
	days1 := GetDayZeroUnixWithZoneAndOffset(ts1, LocalZoneOffset, OffSet) / DaySec
	days2 := GetDayZeroUnixWithZoneAndOffset(ts2, LocalZoneOffset, OffSet) / DaySec
	return int(days2 - days1)
}

// ---------- formatting and parsing ----------

// StringToTime converts a string to time in the fixed local zone.
func StringToTime(date, format string) (time.Time, error) {
	return time.ParseInLocation(format, date, loc)
}

// StringToTimeUnionverts a string to unix seconds in the fixed local zone.
func StringToTimeUnix(date, format string) (int64, error) {
	t, err := StringToTime(date, format)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// TimeToString formats time in the fixed local zone.
func TimeToString(t time.Time, format string) (string, error) {
	return t.In(loc).Format(format), nil
}

// TimestampToString converts unix seconds to formatted time string in the fixed local zone.
func TimestampToString(ts int64, format string) (string, error) {
	return TimeToString(time.Unix(ts, 0), format)
}

// FormatDuration formats a duration in seconds as a human-readable string.
func FormatDuration(ts int64) string {
	days := ts / DaySec
	ts %= DaySec
	hours := ts / HourSec
	ts %= HourSec
	minutes := ts / MinSec
	ts %= MinSec

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d天", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d时", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d分", minutes))
	}
	if ts > 0 {
		parts = append(parts, fmt.Sprintf("%d秒", ts))
	}

	return strings.Join(parts, "")
}
