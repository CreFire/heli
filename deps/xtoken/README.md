# xtoken

## 用途
为服务间和用户校验场景提供对称 AES-GCM token 编码 / 解码器。

## 适用场景
当需要绑定机器 / 用户对的短生命周期 token，并内置过期检查和 AES-GCM 密封时使用。

## 避免使用场景
如果需要非对称签名、比当前 base32 方案更长的 token，或非 AES 加密，则不应使用。

## 关键入口
- `DefaultCoder`：复用的 `TokenCoder`，配置为一周过期。
- `ServerTokenEncode` / `Decode`：编码携带发送方 / 接收方元数据的内部 token。
- `UserTokenEncode` / `Decode`：嵌入用户 / 机器数据，并强制检查机器码子集匹配。
- `NewTokenCoder`：为其他上下文实例化自定义 secret + 过期时间的 coder。

## 注意事项
- 所有编码流程都会调用 `gcmSeal`；解码流程在校验失败时会记录日志并返回错误，例如接收方不匹配、过期、机器变更。

## 业务使用
- `src/service/auth/controller/auth_policy.go` 使用 `UserTokenEncode(gid, deviceId)` 生成返回给客户端的登录会话字符串，然后把该不透明 token 存入在线数据。
- 在业务流程中，该 token 不是唯一事实来源：gate 登录主要会把提交的 session 字符串与缓存在线状态比较。不要把它误解为完全无状态认证层。
- Device ID 是 token 契约的一部分，因此除非有意修改周边业务流程，否则代理不应跨不同 device ID 复用 session。
