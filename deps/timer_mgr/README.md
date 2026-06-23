# timer_mgr
## 用途
通过 `TimerMgr` 和 `CircTimer` 封装定时任务执行。
## 适用场景
当需要可配置间隔和回调注册点的周期性定时器时使用。
## 避免使用场景
如果简单的 `time.Ticker` 已经足够，或依赖外部调度器，则不应使用。
## 关键入口
- `NewTimerMgr` / `NewTimerMgrWithConfig`：用于创建定时循环。
- `TimerMgrConfig`、`TimerFunc`、`CircTimer`：用于定义任务。
- `DefaultConfig`：用于基础配置。
## 注意事项
定时器回调会在 goroutine 中执行；handler 应保持轻量，避免积压。

## 业务使用
- 在这些业务根目录中，该包仅在 gate 测试中被直接导入；生产服务通常通过 `server.MS.TimerMgr` 访问定时器，而不是手动构造 `TimerMgr`。
- 运行时代码假定定时器用于驱动周期性状态上报、缓存清理、刷新任务以及延迟登出 / 离线流程。不要把直接 package import 误解为主要使用面。
