# netmgr
## 用途
TCP 连接管理器，负责客户端 / 服务端队列、事件处理器和消息多路复用。
## 适用场景
当 gate / logic / public 服务需要发起或接受 TCP 连接，并绑定协议处理器时使用。
## 避免使用场景
如果只需要 HTTP / gRPC，或轻量级 goroutine 消息传递，则不应使用；`netmgr` 预期管理长生命周期队列。
## 关键入口
- `NewNetMgr`、`NetMgr` 生命周期（隐含包含 Start / Stop）。
- `NewConnectParams`、`NewListenParams`、`NetOptions`、`MergeOptions`：用于配置队列。
- `MsgQue` 接口和处理器：通过 `msghandler.go` / `msgque.go` 绑定。
- `ConnAgt`：表示带有 `netmgr.IMsgQue` 的连接代理。

## 注意事项
消息顺序和会话处理依赖 `netmgr` 的队列语义；调优请参考 `netmgr/options.go`。

## 业务使用
- `gate`、`public` 和 `logic` 使用 `NetMgr` 作为真实长连接传输层；业务 handler 假定队列 / session 在执行前已经存在，并通过 `netmgr.IMsgQue` 访问。
- `serverlisten.go` 和 `state_auth.go` 中的 `netmgr/options` 会调节服务监听器和机器人客户端的队列行为。不要把这些选项误解为可有可无的修饰项；它们会影响重连和消息处理行为。
- 机器人代码使用与生产 handler 相同的传输路径。因此 bot 流程是预期队列语义的重要参考，而不是独立的假传输层。
