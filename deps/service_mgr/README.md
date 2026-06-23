# service_mgr

## 用途

通过基于 etcd 的 service manager 和进程内实例状态，注册、发现并监控游戏服务。

## 适用场景

需要健康感知 registry、发现 listener，或 gate/logic 节点的服务实例记账时。

## 避免使用场景

运行时已经在其他地方管理注册时。

## 关键入口

- `New` / `NewUnregistered` 用于生产环境 etcd-backed manager。
- `Manager` 查询与选择 API：`List`, `Get`, `PickRandom`, `PickMinOnline`, `PickByHash`。
- `ListenSpec`, `Handler`, `HandlerFunc`, `WatchEvent`, `EventType`。

## 注意事项

Manager 持有对并发敏感的缓存；server 关闭时调用 `Close` 释放 watcher。

- `List(service, nil)` 为高频调用方保留只读快路径。把返回的 slice 视为不可变。

## 业务用法

- 每个有状态服务（`gate`、`logic`、`public`）都会持有一个长生命周期 `Manager` 进行注册/发现。业务代码假定 `server.MS.SvrMgr` 反映当前健康 peer，而不是静态快照。
- auth/login、邮件和战斗流程中的选择逻辑使用发现到的实例作为路由输入。不要把 manager 误读为只负责启动注册；它会直接影响在线业务分发。
- 调用方通常只想要健康/在线实例。如果代理绕过 manager 并硬编码 endpoint，就会跳过已有健康和发现语义。

