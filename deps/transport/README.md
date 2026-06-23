# transport
## 用途
封装 HTTP 和 gRPC 的客户端 / 服务端传输辅助能力，并支持发现和 TLS 选项。
## 适用场景
当拨号或接受 RPC 请求，并需要一致的元数据、超时或服务发现接线时使用。
## 避免使用场景
如果只是调用原始 `net/http` 或 `grpc` 构造函数，且不需要共享 `Transport` 上下文，则不应使用。
## 关键入口
- `Transport`、`ServerTransport`、`NewTransport`、`NewServerTransport`。
- 上下文辅助函数：`NewClientContext`、`FromClientContext`、`NewServerContext`、`FromServerContext`。
- Builder / option API：`ClientOption`、`WithTimeout`、`WithTLSConfig`、`WithDiscovery`、`WithNodeFilter`、`WithSubset`、`WithHealthCheck`、`WithLogger`、`Dial`、`DialInsecure`。
- 发现辅助函数：`NewBuilder`、`WithEndpoint`、`WithOptions`、错误常量（例如 `ErrWatcherCreateTimeout`）。
## 注意事项
Transport 选项应在拨号前完成组合；如果手动创建 discovery watcher，调用方需要负责关闭。

## 业务使用
- 主要被面向 HTTP 的业务代码（`auth_policy`、`query_policy`、`battle_record`）使用，用于在下游 HTTP / gRPC 请求中携带调用方元数据。
- Handler 会读取 `transport.FromClientContext(ctx)`，并假定 gateway / auth 中间件已经注入玩家 / 设备 / 请求上下文。不要把缺失 transport 数据误解为正常分支；现有代码会把它视为意外错误。
- 在本代码库中，`transport` 是上下文 / 元数据桥梁，不是主要的 TCP 游戏传输层。需要与 `netmgr` 明确区分。
