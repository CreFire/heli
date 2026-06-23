# server

## 用途

提供基础 server 串接（`Server`, `GameService`, `MS`），用于编排 TCP/gRPC 服务。

## 适用场景

启动 gate/logic/public/battle server，且需要一致的生命周期检查和死锁检查开关时。

## 避免使用场景

独立 helper 或 library 不依赖共享 `Server` 结构或 deadlock checker 时。

## 关键入口

- `Server` 类型和 `MS` 单例（`var MS = &Server{}`）用于共享状态。
- 接入 `service_module_base.go` 的 `GameService` 接口实现。
- `SetUpDeadlockChecker(enableLockCheck bool)` 用于启用锁监控。

## 注意事项

Server 提供共享 listener/生命周期 hook；保持逻辑模块与 `service_module_base` contract 对齐。

## 业务用法

- 服务入口把 `server.MS` 作为共享进程状态，而不是可随意丢弃的 helper。业务模块读取 `server.MS.NetMgr`、`server.MS.SvrMgr` 或 `server.MS.HttpServe` 时，假定启动阶段已经完成串接。
- `service_module_base` 才是 gate/logic/public 启动背后的真实 contract。不要把 `Server` 误读为承载业务逻辑；它负责生命周期和 transport 脚手架，模块插入其中。
- 测试和持久化 helper 有时会直接替换 `server.MS`。代理应把这视为进程全局变更，而不是隔离的依赖注入。

