# eventbus
## 用途
定义事件总线抽象，用于按可配置路由键分发 `eventpb.Event` 载荷。

## 适用场景
当需要在内存中或通过空实现桩把事件投递给处理器，并保持基于 key 的顺序时使用。

## 避免使用场景
如果已经依赖外部消息队列，或该工作不需要上下文传递，则不应使用。

## 关键入口
- `Bus` 接口、`NewMemoryBus`、`NewNoopBus`
- `Handler` / `KeyFunc` 签名
- `EventHandler` 辅助函数和 `eventpb.Event` 交接

## 注意事项
处理器会在提供的 `async.Async` 实例中运行；需要确保 goroutine 生命周期受控，并在适当时停止总线。
