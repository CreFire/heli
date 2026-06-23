# msg
## 用途
为 C2S / S2S 流程提供轻量级消息包装和 protobuf 解析辅助能力。
## 适用场景
当需要在交给网络层 / 服务层之前构建、包装或解析协议内消息（`pb.MSG_ID`）时使用。
## 避免使用场景
如果调用方处理的是原始 protobuf 结构体，或只需要游戏协议之外的 protobuf 序列化，则不应使用。
## 关键入口
- `Message` / `NewMsg*` 系列：用于构造带错误码或内嵌 proto body 的响应。
- `PBParser` 及其 `PBPack` / `PBUnPack`：用于序列化 / 反序列化载荷。
- `IMsgParser` / `ParseFunc` 组合：供 `MsgParser` 实例化消息体。
## 注意事项
解析器注册位于 `parser.go` / `parser_pb.go`；消息头假定 `HEAD_SIZE=2`，传输层必须遵守该约定。

## 业务使用
- 在 gate / robot / logic / public handler 中作为 C2S、S2S 和 RPC 载荷的通用信封。业务代码假定 `msgId` 和载荷类型已经匹配注册的解析器。
- `service/logic/module/*/handler.go` 中的调用方通常会在解析后直接断言请求类型。不要把 `msg.Message` 误解为防御边界；周边代码期望协议正确，类型不匹配时可能直接崩溃。
- `NewRspMsgWithProtoAndCode` 用于协议响应，而不是通用错误传输。应保留游戏协议语义，不要把 HTTP / gRPC 风格错误处理混入这一层。
