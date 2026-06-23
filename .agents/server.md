# heli 服务器项目文档

> 项目路径：`E:\work\heli\server`  
> Go module：`game`  
> 当前定位：Go TCP 长连接游戏服务器，使用 Protobuf 协议，依赖 MongoDB 与 Redis。  
> 最近同步：已执行 `codegraph sync`，退出码 `0`。

---

## 1. 项目概览

`server` 是 `heli` 项目的服务端工程，当前已具备最小联机闭环的核心代码：

- TCP 长连接服务器。
- 大端序二进制包头协议。
- Protobuf 消息体编解码。
- 登录/创建玩家。
- 简单匹配组房。
- 玩家操作广播为游戏快照。
- MongoDB 持久化玩家与房间数据。
- Redis 写入房间运行状态。
- Viper 配置加载与环境变量覆盖。

当前 `src/main.go` 仍为空入口，仅包含 `package main`。`README.md` 中写的是 `go run ./cmd/server`，但当前工程未发现 `cmd/server` 目录，需要补齐启动入口或修正文档中的启动命令。

---

## 2. 当前目录结构

```text
server/
├── .agents/
│   └── server.md                  # 本文档
├── bin/                           # 二进制或运行产物目录
├── conf/
│   └── local.yaml                 # 本地配置
├── deps/                          # 依赖资源目录
├── docconf/                       # 文档/配置辅助目录
├── internal/
│   ├── config/
│   │   ├── config.go              # 配置加载
│   │   └── config_test.go         # 配置测试
│   ├── game/
│   │   └── manager.go             # 登录、匹配、房间、快照管理
│   ├── netserver/
│   │   └── server.go              # TCP 服务与会话处理
│   ├── protocol/
│   │   ├── codec.go               # TCP 包读写
│   │   ├── commands.go            # 指令号定义
│   │   └── game.pb.go             # 生成的 Protobuf 代码
│   └── store/
│       └── store.go               # MongoDB/Redis 连接管理
├── src/
│   ├── main.go                    # 当前入口占位
│   └── proto/pb/                  # 另一份生成的 Protobuf 代码
├── tools/
│   ├── bin/                       # protoc 与 Go 生成插件
│   ├── proto/                     # proto 源文件
│   ├── server_gen_proto.py        # 协议生成脚本
│   └── export_proto_common.py     # 协议导出辅助脚本
├── go.mod
├── go.sum
└── README.md
```

---

## 3. 技术栈

| 类型 | 选型 |
| --- | --- |
| 语言 | Go |
| 网络 | TCP 长连接 |
| 协议 | Protobuf |
| 配置 | Viper |
| 数据库 | MongoDB |
| 缓存 | Redis |
| 日志 | 当前主要使用标准库 `log`，`go.mod` 已引入 `zap` |
| 测试 | Go test + testify |

主要依赖见 `go.mod`：

- `github.com/spf13/viper`
- `github.com/redis/go-redis/v9`
- `go.mongodb.org/mongo-driver`
- `google.golang.org/protobuf`
- `go.uber.org/zap`
- `github.com/stretchr/testify`

---

## 4. 配置说明

默认配置文件：`conf/local.yaml`

```yaml
tcp_addr: ":7001"
mongo_uri: "mongodb://127.0.0.1:27017"
mongo_db: "heli"
redis_addr: "127.0.0.1:6379"
redis_password: ""
redis_db: 0
room_size: 2
tick_interval: "200ms"
```

配置加载逻辑位于 `internal/config/config.go`：

1. 设置默认值。
2. 优先读取环境变量 `CONFIG_FILE` 指定的配置文件。
3. 未指定时读取 `conf/local.yaml` 或当前目录下的 `local.yaml`。
4. 支持环境变量覆盖，`.` 会映射为 `_`。

常用环境变量：

```powershell
$env:TCP_ADDR=":7001"
$env:MONGO_URI="mongodb://127.0.0.1:27017"
$env:MONGO_DB="heli"
$env:REDIS_ADDR="127.0.0.1:6379"
$env:REDIS_PASSWORD=""
$env:REDIS_DB="0"
$env:ROOM_SIZE="2"
$env:TICK_INTERVAL="200ms"
$env:CONFIG_FILE="E:\work\heli\server\conf\local.yaml"
```

---

## 5. 启动依赖

本地运行前需要准备：

- MongoDB：`127.0.0.1:27017`
- Redis：`127.0.0.1:6379`

连接逻辑位于 `internal/store/store.go`：

- MongoDB 连接超时：`5s`
- Redis 连接超时：共用 `5s` 上下文
- 连接成功后返回 `Stores{Mongo, DB, Redis}`
- `Close` 会关闭 MongoDB 与 Redis 连接

---

## 6. 网络协议

协议实现位于 `internal/protocol/codec.go`。

### 6.1 TCP 包格式

大端序：

```text
uint32 body_length + uint16 cmd + protobuf_body
```

字段说明：

| 字段 | 长度 | 说明 |
| --- | --- | --- |
| `body_length` | 4 字节 | Protobuf 消息体长度 |
| `cmd` | 2 字节 | 指令号 |
| `protobuf_body` | N 字节 | Protobuf 序列化后的消息体 |

### 6.2 协议限制

```go
const (
    HeaderSize    = 6
    MaxPacketSize = 1 << 20
)
```

单包最大消息体：`1 MiB`。

---

## 7. 指令清单

指令定义位于 `internal/protocol/commands.go`。

| Cmd | 消息 | 方向 | 说明 |
| --- | --- | --- | --- |
| `1001` | `LoginReq` | Client -> Server | 登录/创建玩家 |
| `1002` | `LoginResp` | Server -> Client | 登录响应 |
| `1101` | `MatchReq` | Client -> Server | 加入匹配队列 |
| `1102` | `MatchResp` | Server -> Client | 匹配中/匹配成功 |
| `1201` | `PlayerOp` | Client -> Server | 玩家战斗操作 |
| `1202` | `GameSnapshot` | Server -> Client | 帧快照/操作广播 |
| `9001` | `Heartbeat` | 双向 | 心跳 |

---

## 8. 当前联机闭环

当前核心流程：

1. 客户端建立 TCP 连接。
2. 客户端发送 `LoginReq`。
3. 服务端在 MongoDB `players` 集合中 upsert 玩家数据。
4. 客户端发送 `MatchReq`。
5. 服务端按 `room_size` 将等待队列中的玩家组房。
6. 服务端在 MongoDB `rooms` 集合中插入房间记录。
7. 服务端向 Redis 写入 `room:{room_id}:status = playing`。
8. 客户端发送 `PlayerOp`。
9. 服务端将操作加入房间操作队列。
10. 服务端生成 `GameSnapshot` 并广播给房间内所有会话。

---

## 9. 模块说明

### 9.1 `internal/netserver`

职责：

- 监听 TCP 地址。
- 接收客户端连接。
- 为每个连接创建 `Session`。
- 分离读循环与写循环。
- 根据 `cmd` 分发处理逻辑。

当前会话处理逻辑：

- `CmdLoginReq`：反序列化 `LoginReq`，调用 `game.Manager.Login`。
- `CmdMatchReq`：要求已登录，调用 `game.Manager.Match`。
- `CmdPlayerOp`：写入房间操作并广播快照。
- `CmdHeartbeat`：回包当前毫秒时间戳。
- 未知指令：打印日志后忽略。

### 9.2 `internal/game`

职责：

- 玩家登录。
- 匹配等待队列。
- 房间创建和内存管理。
- 玩家操作暂存。
- 快照生成。

关键内存结构：

- `waiting []Session`：匹配等待队列。
- `rooms map[string]*Room`：房间表。
- `playerRoom map[string]string`：玩家所在房间索引。

### 9.3 `internal/store`

职责：

- 连接 MongoDB。
- 连接 Redis。
- 聚合存储依赖并提供关闭方法。

### 9.4 `internal/config`

职责：

- 读取配置文件。
- 设置默认配置。
- 支持环境变量覆盖。

---

## 10. 本地开发命令

### 10.1 安装/整理依赖

```powershell
go mod tidy
```

### 10.2 运行测试

```powershell
go test ./...
```

### 10.3 启动服务

当前代码尚未补齐可用启动入口。推荐补齐以下入口之一：

方案 A：新增 `cmd/server/main.go`，保持 `README.md` 中的命令不变：

```powershell
go run ./cmd/server
```

方案 B：将 `src/main.go` 改为真实入口，并将启动命令修正为：

```powershell
go run ./src
```

建议优先使用方案 A，符合 Go 服务端常见布局。

---

## 11. Proto 生成

协议源文件位于：

```text
tools/proto/
├── common.proto
├── client.proto
└── micro.proto
```

当前仓库包含：

- `tools/server_gen_proto.py`
- `tools/export_proto_common.py`
- `tools/bin/protoc.exe`
- `tools/bin/protoc-gen-go.exe`
- `tools/bin/protoc-gen-go-grpc.exe`
- `tools/bin/protoc-gen-gotag.exe`
- `tools/bin/protoc-go-inject-tag.exe`

生成产物目前存在两处：

```text
internal/protocol/game.pb.go
src/proto/pb/*.pb.go
```

需要后续统一生成目标，避免协议代码重复和导入路径混乱。

---

## 12. 已知问题与待办

### 12.1 高优先级

- [ ] 补齐真实启动入口：建议新增 `cmd/server/main.go`。
- [ ] 修正 `README.md` 与实际目录结构不一致的问题。
- [ ] 明确 `src/proto/pb` 与 `internal/protocol` 两套 pb 代码的用途，并统一协议生成路径。
- [ ] 为 TCP server 增加优雅停机流程和连接清理。
- [ ] 为 `PlayerOp` 增加登录态、房间归属和玩家权限校验。

### 12.2 中优先级

- [ ] 使用 `zap` 替换标准库 `log`，统一结构化日志。
- [ ] 增加连接读写超时、心跳超时和空闲连接清理。
- [ ] 增加包频率限制和恶意包防护。
- [ ] 为匹配队列增加去重逻辑，避免同一会话重复排队。
- [ ] 为 MongoDB `players`、`rooms` 建索引。

### 12.3 后续玩法

- [ ] 怪物波次。
- [ ] 塔属性与升级规则。
- [ ] 战斗结算。
- [ ] 断线重连。
- [ ] 房间恢复。
- [ ] 多房间定时 Tick。
- [ ] 战斗日志或回放。

---

## 13. 推荐启动入口草案

建议新增 `cmd/server/main.go`，逻辑如下：

1. 加载配置。
2. 连接 MongoDB/Redis。
3. 创建 `netserver.Server`。
4. 监听系统信号。
5. 启动 TCP 服务。
6. 收到退出信号后关闭依赖。

推荐伪流程：

```text
cfg := config.Load()
stores := store.Connect(ctx, cfg)
srv := netserver.New(cfg, stores)
srv.Run(ctx)
```

---

## 14. 验收清单

最小可运行版本建议满足：

- [ ] `go test ./...` 通过。
- [ ] `go run ./cmd/server` 可启动。
- [ ] 缺少 MongoDB/Redis 时能输出明确错误。
- [ ] 客户端可成功 TCP 连接 `:7001`。
- [ ] 发送 `LoginReq` 可收到 `LoginResp`。
- [ ] 两个客户端发送 `MatchReq` 可收到 `matched`。
- [ ] 房间内发送 `PlayerOp` 可收到 `GameSnapshot`。
- [ ] 发送 `Heartbeat` 可收到心跳回包。

---

## 15. 维护说明

更新本文档时优先基于当前代码事实：

1. 先检查 `go.mod`、`README.md`、`conf/local.yaml`。
2. 再检查 `internal/config`、`internal/netserver`、`internal/game`、`internal/store`、`internal/protocol`。
3. 若修改了启动方式、协议指令、配置字段或依赖，必须同步更新本文档。
4. 若新增 proto 生成脚本或改变生成目标，必须同步更新第 11 节。
