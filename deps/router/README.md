# router
## 用途
通过配置好的 RPC manager，把传入消息路由到 RPC、服务或自定义 handler。
## 适用场景
当需要把 `rpcmgr.RpcMgr` 接入请求处理，并为 C2S / S2S 流程暴露 handler 工厂时使用。
## 避免使用场景
如果逻辑位于 gate / public / logic 管线之外，且不需要 `Router` 抽象，则不应使用。
## 关键入口
- `NewRouter(*rpcmgr.RpcMgr)`：用于构建 router。
- `C2SHandlerFunc`、`S2SHandlerFunc` 以及注册到 `Router` map 中的消息分发项。
## 注意事项
Router 接线发生在更高层模块（gate / logic）中；handler 实现需要与 netmgr 队列保持一致。

## 业务使用
- Gate / public / logic 的模块管理器会为每个服务角色构建一个 router，并按协议消息注册 handler。业务代码假定路由是显式且集中注册的，而不是基于约定自动发现。
- 在 logic handler 中，`router.C2SHandlerFunc` 位于解析之后、模块业务逻辑之前；不要把它误解为校验层。大多数语义检查发生在模块 handler 内部。
- Router 归属遵循服务边界。gate router 不是通用跨服务总线；代理不应把 gate / public / logic handler 混入同一个注册表。
