# rpc_mgr
## 用途
基于 netmgr 队列管理 RPC 客户端 / 服务端，并提供超时控制和响应处理。
## 适用场景
当需要在服务 / gate 之间创建 RPC 流程，并编排重试或失败处理时使用。
## 避免使用场景
如果上下文不需要 RPC 抽象（纯 HTTP / gRPC），或逻辑只发布事件，则不应使用。
## 关键入口
- `NewRpcMgr(nm *netmgr.NetMgr)`：用于构建 manager。
- `RpcTaskAddFailed`：用于重试记账。
- `RpcRequestHandler`、`RpcResponseHandler`、`RpcCallInfo`：用于请求生命周期。
- 超时常量：`RPC_SERVER_HANDLE_TIMEOUT`、`RPC_CLIENT_WAIT_TIMEOUT`。
## 注意事项
Manager 预期使用长生命周期 netmgr 连接；分发重型操作前应调整超时。

## 业务使用
- Gate / public / logic 模块使用 `rpc_mgr` 作为长连接 TCP 链路上的内部请求 / 响应路径，尤其用于那些需要先请求其他服务再回复客户端的 handler。
- 调用方假定 RPC 已经绑定到 `netmgr` 和服务发现。不要把它误解为独立的通用 RPC 框架；它依赖本项目的消息 ID、队列和服务拓扑。
- Handler 代码经常把 RPC 失败视为需要立即暴露的业务失败。代理应让超时和响应类型与现有请求 handler 保持一致，而不是添加兼容层。
