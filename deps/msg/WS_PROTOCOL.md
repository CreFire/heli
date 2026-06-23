# WebSocket 协议说明

本文档说明当前框架 WebSocket 连接方式与二进制消息协议。相关实现主要位于：

- `network/ws_server.go`
- `network/ws_client.go`
- `network/ws_conn.go`
- `network/tcp_msg.go`
- `network/cstruct/cstruct.go`
- `gate/gate.go`
- `gate/agent.go`

## 1. 连接方式

框架使用 `github.com/gorilla/websocket` 实现 WebSocket。

### 1.1 服务端

服务端由 `network.WSServer` 提供，在 `gate.Gate.Run()` 中，当 `Gate.WSAddr` 不为空时启动。

主要配置项：

| 配置 | 说明 |
| --- | --- |
| `WSAddr` | WebSocket 监听地址 |
| `MaxConnNum` | 最大连接数 |
| `PendingWriteNum` | 每个连接的异步写队列长度 |
| `HTTPTimeout` | HTTP 握手读写超时时间 |
| `CertFile` | TLS 证书文件 |
| `KeyFile` | TLS 私钥文件 |
| `LenMsgLen` | 消息长度字段占用字节数，可为 1、2、4 |
| `MinMsgLen` | 最小消息长度 |
| `MaxMsgLen` | 最大消息长度 |
| `LittleEndian` | 是否使用小端序 |

启动逻辑：

- 未配置 `CertFile` / `KeyFile`：启动 `ws://`
- 配置 `CertFile` / `KeyFile`：启动 `wss://`

服务端使用 HTTP `GET` 请求升级为 WebSocket。

当前 `websocket.Upgrader` 的 `CheckOrigin` 固定返回 `true`，即不限制跨域来源。

```go
CheckOrigin: func(_ *http.Request) bool { return true }
```

当前服务端没有绑定固定路径，`http.Server.Handler` 直接使用 `WSHandler`，因此所有进入该监听地址的 `GET` 请求都会尝试升级为 WebSocket。

### 1.2 客户端

客户端由 `network.WSClient` 提供，使用 `websocket.Dialer` 连接服务端。

主要配置项：

| 配置 | 说明 |
| --- | --- |
| `Addr` | WebSocket 连接地址，例如 `ws://127.0.0.1:3653` |
| `ConnNum` | 连接数量 |
| `ConnectInterval` | 重连间隔 |
| `PendingWriteNum` | 每个连接的异步写队列长度 |
| `HandshakeTimeout` | 握手超时时间 |
| `AutoReconnect` | 是否自动重连 |
| `LenMsgLen` | 消息长度字段占用字节数 |
| `MinMsgLen` | 最小消息长度 |
| `MaxMsgLen` | 最大消息长度 |
| `LittleEndian` | 是否使用小端序 |

连接地址示例：

```text
ws://127.0.0.1:3653
ws://127.0.0.1:3653/ws
wss://example.com/game
```

## 2. 传输方式

WebSocket 层只使用二进制消息：

```go
conn.WriteMessage(websocket.BinaryMessage, b)
```

读取时使用：

```go
_, b, err := conn.ReadMessage()
```

因此客户端必须发送 WebSocket BinaryMessage，不能直接发送文本消息。

## 3. 协议总览

WebSocket BinaryMessage 内部承载框架自定义二进制包。

完整格式：

```text
| len | checkCode | msgType | cmd | rpcCallId? | body |
| len 2 | cbCheckCode 1 | msg type 1 | main cmd 2 | sub cmd 2 | rpc call id 4(该字段根据msg type是否包含 MSG_TYPE_RPC 可选) | data |
```

其中 `rpcCallId` 只有当 `msgType` 包含 `MSG_TYPE_RPC` 标记时才存在。

更详细的结构：

```text
| len N字节 | checkCode 1字节 | msgType 1字节 | mainCmdID 2字节 + subCmdID 2字节 | rpcCallId 4字节可选 | body N字节 |
```

注意：`cmd` 在代码中作为一个 `uint32` 读写，不是分别写入两个 `uint16`。

## 4. 外层包格式

外层包由 `network.MsgParser` 负责。

### 4.1 len

`len` 表示后续数据长度，不包含 `len` 字段自身，但包含 `checkCode`。

默认配置：

```go
lenMsgLen = 2
minMsgLen = 1
maxMsgLen = 4096
littleEndian = true
```

`LenMsgLen` 可配置为：

| 值 | 最大长度 |
| ---: | ---: |
| 1 | `math.MaxUint8` |
| 2 | `math.MaxUint16` |
| 4 | `math.MaxUint32` |

### 4.2 checkCode

`checkCode` 是 1 字节校验码。

生成逻辑：

1. 对 `msgType + cmd + rpcCallId? + body` 的所有字节求和。
2. 写入 `^sum + 1` 作为 `checkCode`。
3. 接收方校验 `checkCode + payload` 的所有字节累加结果是否为 `0`。

发送端逻辑位于 `MsgParser.Pack()`：

```go
msg[l] = ^cbCheckCode + 1
```

接收端校验逻辑：

```go
if cbCheckCode != 0 {
    return nil, fmt.Errorf("cstruct: read checkCode err")
}
```

## 5. 业务头格式

`checkCode` 后面的业务数据由 `network/cstruct.Processor` 负责解析。

普通消息：

```text
| msgType 1字节 | cmd 4字节 | body |
```

RPC 消息：

```text
| msgType 1字节 | cmd 4字节 | rpcCallId 4字节 | body |
```

### 5.1 msgType

定义在 `network/cstruct/cstruct.go`：

```go
const (
    MSG_TYPE_NONE uint8 = 0
    MSG_TYPE_RPC  uint8 = 0x01
)
```

| 类型 | 值 | 说明 |
| --- | ---: | --- |
| `MSG_TYPE_NONE` | `0` | 普通消息 |
| `MSG_TYPE_RPC` | `0x01` | RPC 消息，额外携带 `rpcCallId` |

`msgType` 按位标记使用：

```go
FlagGet(msgType, MSG_TYPE_RPC)
```

因此后续可以继续扩展其他 bit flag。

### 5.2 cmd

`cmd` 是 4 字节命令 ID，由 `mainCmdID` 和 `subCmdID` 组合而成。

组合方式：

```go
func MakeDWORD(mainCmdID uint16, subCmdID uint16) uint32 {
    return uint32(mainCmdID) | uint32(subCmdID)<<16
}
```

拆分方式：

```go
func GetCmd(CmdID uint32) (mainCmdID, subCmdID uint16) {
    mainCmdID = uint16(CmdID)
    subCmdID = uint16(CmdID >> 16)
    return
}
```

即：

```text
cmd = mainCmdID低16位 | subCmdID高16位
```

默认小端序下，网络字节排列为：

```text
mainCmdID low byte
mainCmdID high byte
subCmdID low byte
subCmdID high byte
```

### 5.3 rpcCallId

当 `msgType & MSG_TYPE_RPC != 0` 时，`cmd` 后额外存在 4 字节 `rpcCallId`。

如果 `msgType` 非 0 但 `rpcCallId` 为 0，当前实现会记录错误日志，但不会直接拒绝解析。

## 6. body 编码

消息体支持两种编码方式。

### 6.1 protobuf

如果注册的消息实现了：

```go
proto.Message
```

则使用 protobuf 编解码：

```go
proto.Marshal()
proto.Unmarshal()
```

### 6.2 cstruct

如果不是 protobuf 消息，则使用项目内的 `cstruct` 编解码：

```go
cstruct.Marshal()
cstruct.Unmarshal()
```

### 6.3 空 body

注册消息时允许传入 `nil`，表示该命令没有消息体。

解析后 `RecvMsg.Msg` 为 `nil`。

## 7. 收包流程

服务端收包流程：

```text
WebSocket BinaryMessage
        ↓
WSConn.ReadMsg()
        ↓
MsgParser.ReadWs()
        ↓
校验 len / checkCode
        ↓
去掉 len 和 checkCode
        ↓
Processor.Unmarshal()
        ↓
解析 msgType / cmd / rpcCallId / body
        ↓
Processor.Route()
        ↓
msgHandler 或 chanrpc router
```

对应代码：

- `gate/agent.go`
- `network/ws_conn.go`
- `network/tcp_msg.go`
- `network/cstruct/cstruct.go`

## 8. 发包流程

业务层调用：

```go
agent.WriteMsg(recv, mainCmdID, subCmdID, msg)
```

发包流程：

```text
Processor.Marshal()
        ↓
生成 msgType + cmd + rpcCallId? + body
        ↓
MsgParser.Pack()
        ↓
添加 len + checkCode
        ↓
WSConn.Write()
        ↓
websocket.BinaryMessage
```

## 9. 示例

### 9.1 普通消息

条件：

- `LenMsgLen = 2`
- `LittleEndian = true`
- `msgType = MSG_TYPE_NONE`
- `mainCmdID = 1`
- `subCmdID = 2`

命令 ID：

```go
MakeDWORD(1, 2) == 0x00020001
```

包结构：

```text
| len 2字节 | checkCode 1字节 | msgType 1字节 | cmd 4字节 | body |
```

其中小端序下 `cmd` 字节为：

```text
01 00 02 00
```

### 9.2 RPC 消息

条件：

- `msgType = MSG_TYPE_RPC`
- `rpcCallId != 0`

包结构：

```text
| len 2字节 | checkCode 1字节 | msgType 1字节 | cmd 4字节 | rpcCallId 4字节 | body |
```

## 10. 注意事项

1. WebSocket 内部仍然保留 `len` 字段。  
   虽然 WebSocket 本身有消息边界，但框架为了复用 TCP 的 `MsgParser`，仍然使用自定义长度头。

2. 只支持 BinaryMessage。  
   文本 WebSocket 客户端不能直接与该协议通信。

3. 服务端默认不校验 Origin。  
   `CheckOrigin` 固定返回 `true`，生产环境如有浏览器客户端，需要额外考虑安全策略。

4. 服务端没有固定 path。  
   当前 `WSServer` 直接把 `WSHandler` 作为整个 HTTP Server 的 Handler。

5. `ReadWs()` 对过短 WebSocket frame 防护不足。  
   如果收到长度小于 `LenMsgLen` 的二进制帧，当前实现可能因为切片越界而 panic。

6. `MaxMsgLen` 同时影响 WebSocket 读取限制和协议包长度校验。

## 11. 协议简表

默认情况下，完整二进制包为：

```text
普通消息：
| len:uint16 little-endian | checkCode:uint8 | msgType:uint8 | cmd:uint32 little-endian | body |

RPC 消息：
| len:uint16 little-endian | checkCode:uint8 | msgType:uint8 | cmd:uint32 little-endian | rpcCallId:uint32 little-endian | body |
```

默认参数：

| 参数 | 默认值 |
| --- | ---: |
| `LenMsgLen` | `2` |
| `MinMsgLen` | `1` |
| `MaxMsgLen` | `4096` |
| `LittleEndian` | `true` |
| WebSocket message type | BinaryMessage |
| body 编码 | protobuf 或 cstruct |
