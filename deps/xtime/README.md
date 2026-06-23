# xtime

## 用途
提供标准化的时区感知辅助函数，覆盖默认北京时间区域、偏移、格式化和边界检查。

## 适用场景
当需要一致的“本地”时间戳、带偏移的刷新边界（日 / 周 / 月），或锚定固定时区的字符串格式化 / 解析时使用。

## 避免使用场景
如果需要超出全局偏移之外的用户可配置时区，或标准 `time` 包调用已经足够，则不应使用。

## 关键入口
- `SetLocalZoneOffset`、`SetGmAdd`：调整固定时区偏移和 GM 附加偏移。
- `Now`、`NowUnix`、`NowUnixMs`：应用覆盖后的当前时间。
- `GetLocal*ZeroUnix` / `NextLocalDayZeroSec`：计算带偏移的日 / 周 / 月重置点。
- `CheckDaily*` / `Weekly*` / `Monthly*Fresh`：判断两个时间戳是否跨过刷新边界。
- `FormatDuration`、`StringToTime*`、`TimeToString`、`TimestampToString`：在字符串和时间戳之间转换。

## 注意事项
- 时区逻辑始终使用 `LocalZoneOffset` 加 `OffSet`（默认 4 小时）计算刷新点；`GetLocalWeekZeroUnix` 假定周一为一周开始。

## 业务使用
- Auth / query / logic / public 代码使用 `xtime` 作为服务端规范时钟，用于登录时间戳、缓存清理定时器、每日刷新检查以及邮件 / 排行重置边界。
- 许多玩法检查会调用 `CheckDailyFresh` 和 `GetLocalDayZeroUnixWithOffSet`；调用方假定使用服务端配置的重置时间，而不是玩家设备时区。不要从 auth 响应中推断出按用户时区处理的语义。
- `MinFastIdAt` 和每日重置逻辑在全局邮件、worldboss 等模块中存在耦合；修改重置解释会改变业务数据窗口。
