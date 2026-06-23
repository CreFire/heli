# basal
## 用途
提供代码库中通用的基础类型转换、哈希原语和小型工具函数。

## 适用场景
当需要字符串 / 数字转换、JSON 辅助、 一致性哈希、进程内通道、有序集合，或 `NextNumber` 这类小型监控工具时使用。

## 避免使用场景
如果调用方已有更具体的工具函数，应避免与其他辅助工具栈重复实现相同功能。

## 关键入口
- `ToInt64`、`ToFloat64`、`ToJsonString`、`ConvertInt32`、`AtoInt`
- `ConsistentHash[T NodeKey]` / `NewConsistentHash` 及其 `HashFunc`
- `Chan[T]`、`NewChan`、`SortedList`、`SortedSet`、`NextNumber`

## 注意事项
`ConsistentHash` 要求调用方实现 `NodeKey`，以保证 `GetKey` 与 `HashFunc` 的语义保持一致。

## 业务使用
- 在 `logic`、`public`、`robot` 和 `auth/login_queue` 中，业务代码主要依赖 `SafeGo` / `SafeRun`、`MonitorMgr`、`NoCacheLineData`，以及 `Min` 这类小型数值辅助函数。
- 调用方把 `SafeGo` / `SafeRun` 当作旁路 goroutine 的 panic 隔离手段，而不是重试或错误传播机制。不要推断失败会被回传给调用方。
- `NoCacheLineData` 用于队列和在线流程中的热点计数器 / 时间戳；如果路径对延迟敏感，不要替换成普通包装对象。
- 不要把该包过度理解为通用框架。在业务代码中，它主要是围绕并发和数值夹取的“小型运行时胶水”。
