# proto
## 用途
暴露 S2S / C2S / RPC 层使用的生成版 protobuf 定义和共享类型，例如 `pb.MSG_ID`。
## 适用场景
当需要引用协议 ID、共享结构（`MsgHead`、`MsgType`、`MsgFlags`），或关联 `msgbase` 等子包时使用。
## 避免使用场景
当 `.proto` 源文件发生变化时，应运行 `tools/build_proto.bat`，不要直接编辑生成文件。
## 关键入口
- `pb.common`、`rpc` 等路径下的 `pb` 枚举 / 类型，由 `../../Proto` 生成。
- `MsgType`、`MsgHead`、`MsgFlags`：用于网络消息头。
- 子模块 `msgbase` 承载更底层的共享定义。
## 注意事项
将该包视为只读；每当 `.proto` 文件变更时都应重新生成，保证 import 同步。
