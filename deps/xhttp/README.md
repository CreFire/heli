# xhttp
## 用途
围绕 Gin、基于 proto 的 handler，以及标准化 HTTP 错误映射，提供 HTTP 服务 / 代理辅助能力。

## 适用场景
当需要暴露 protobuf 风格的 HTTP 端点、运行小型 Gin 服务，或通过 `service_mgr` 反向代理请求时使用。

## 避免使用场景
如果只需要普通 `net/http`、大型自定义中间件栈，或非 proto 请求路由，则不应使用。

## 关键入口
- `HttpServer`
- `HandlerMgr`、`ProtoMessageHandler`
- `HttpError`、`NewHttpError`
- `InitProxy`、`ReverseProxy`
- `CORSMiddleware`

## 注意事项
`HttpServer` 包装 Gin 生命周期（`Init`、`StartServe`、`StopServe`）。Proto handler 会强制执行特定包的请求解析和 token 处理。

## 业务使用
- `auth` 和 `query` 使用 `xhttp` 作为 protobuf-over-HTTP 边界。业务 handler 返回 `*xhttp.HttpError`，因此调用方假定协议解码、token 提取和错误格式化已经在此完成。
- `auth_policy.go` 和 `query_policy.go` 会把请求类型不匹配或缺少 transport 元数据视为硬失败。不要把这些 handler 误解为宽容的 JSON 端点；它们期望内部 protobuf 契约。
- `authsvr.go` 和 `query` 中的 `InitProxy` / `ReverseProxy` 是服务路由辅助工具，而不是通用 API 网关层。它们依赖 `service_mgr` 命名和当前内部拓扑。

