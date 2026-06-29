# kitex

## 用途

为当前项目提供最小可并存的 Kitex 接入层，不替换现有 `deps/server`、`deps/rpc_mgr`、`deps/service_mgr`。

## 当前策略

- 先做并存接入，不改现有 gRPC/TCP 主链路。
- 通过独立 demo server/client 验证 Kitex 依赖、代码生成、启动与调用闭环。
- 后续真实业务服务如需迁移，再按单服务逐步替换。
