# selector
## 用途
通过可插拔的负载均衡器、过滤器和构建器，为 RPC 客户端抽象节点 / peer 选择逻辑。
## 适用场景
当通过 transport builder 拨号服务，并需要过滤、再均衡或基于 WRR 的节点选择时使用。
## 避免使用场景
如果只选择硬编码端点，或直接依赖 Kubernetes 服务发现，则不应使用。
## 关键入口
- `NewBuilder` / `Builder` / `Selector`：用于创建选择管线（`selector.Builder`、`selector.Selector`）。
- `Selector` 过滤器，例如 `Version`、`NodeFilter`、`WithNodeFilter`。
- `Option`、`SelectOptions` 以及 `GlobalSelector` / `SetGlobalSelector`：用于全局覆盖。
- 具体实现：`DefaultNode`、`Balancer`、`Peer`，以及 `selector` 子包下的 WRR builder。
## 注意事项
全局 selector 状态是共享的；`SetGlobalSelector` 调用应靠近初始化阶段，避免竞态。
