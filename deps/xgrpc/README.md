# xgrpc
## 用途
提供共享的 gRPC 客户端连接管理器，以及一元调用重试拦截器。

## 适用场景
当希望按地址复用出站 gRPC 连接，并对一元调用应用标准重试策略时使用。

## 避免使用场景
如果需要流式重试、自定义发现，或每次调用都独占连接，则不应使用。

## 关键入口
- `GrpcConnMgr`、`ConnManager`
- `NewConnManager`
- `ConnManager.GetConn`、`CloseAll`
- `RetryOptions`、`DefaultRetryOptions`
- `UnaryRetryInterceptor`

## 注意事项
连接会按地址缓存。关闭服务时需要显式关闭；重试只包装一元客户端 RPC。

## 业务使用
- 当前业务使用范围较窄：`service/logic/module/battle/battle_module.go` 使用 `GrpcConnMgr.GetConn` 调用 battle server 进行校验。
- 调用方假定连接按地址复用，并且远端已经由服务发现提前稳定选定。不要把 `xgrpc` 误解为主要内部 RPC 路径；大多数集群内流量仍然通过 `rpc_mgr` / `netmgr`。


