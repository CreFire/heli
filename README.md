# heli cooperative tower-defense server

最小版本：Go + TCP 长连接 + protobuf 协议 + MongoDB + Redis。

## 配置

配置解析使用 Viper。

默认读取：

```text
conf/local.yaml
```

也可以通过环境变量覆盖，例如：

```powershell
$env:TCP_ADDR=":7001"
$env:MONGO_URI="mongodb://127.0.0.1:27017"
$env:MONGO_DB="heli"
$env:REDIS_ADDR="127.0.0.1:6379"
$env:ROOM_SIZE="2"
```

指定配置文件：

```powershell
$env:CONFIG_FILE="E:\work\heli\server\conf\local.yaml"
```

兼容环境变量样例见 `conf/local.env`。

## 启动依赖

- MongoDB: `127.0.0.1:27017`
- Redis: `127.0.0.1:6379`

## 运行

```powershell
go mod tidy
go run ./cmd/server
```

默认监听：`TCP :7001`。

## 生成 Proto

本项目使用 `tools/bin` 内置的 `protoc` 和 `protoc-gen-go`。

```powershell
powershell -ExecutionPolicy Bypass -File tools\gen-proto.ps1
```

生成目标：

```text
internal/protocol/game.pb.go
```

## TCP 包格式

大端序：

```text
uint32 body_length + uint16 cmd + protobuf_body
```

## MVP 指令

| Cmd | 消息 | 说明 |
| --- | --- | --- |
| 1001 | LoginReq | 登录/创建玩家 |
| 1002 | LoginResp | 登录响应 |
| 1101 | MatchReq | 加入匹配 |
| 1102 | MatchResp | 匹配中/匹配成功 |
| 1201 | PlayerOp | 玩家战斗操作，例如放置/升级塔 |
| 1202 | GameSnapshot | 帧快照/操作广播 |
| 9001 | Heartbeat | 心跳 |
| 501 | BattleJoinREQ | 战斗服直连后进入房间，携带 room_id 与 battle_token |
| 502 | BattleJoinRSP | 战斗服进房响应，可携带当前快照 |
| 503 | BattleOpREQ | 局内操作：建塔、魔力重随、合成、购买矿工 |
| 504 | BattleOpRSP | 局内操作结果，返回 op_id、server_tick、tower_id/miner_id |
| 505 | BattleSnapshotNTF | 战斗服下发完整状态快照 |
| 506 | BattleDeltaNTF | 战斗服下发状态增量事件 |

## 当前玩法闭环

1. 客户端 TCP 连接。
2. 发送 `LoginReq`，服务端写入/更新 Mongo `players`。
3. 发送 `MatchReq`，服务端按 `ROOM_SIZE` 人数组房，房间状态写入 Mongo/Redis。
4. 客户端发送 `PlayerOp`，服务端按房间广播 `GameSnapshot`。

这是最小联机闭环；怪物波次、塔属性、结算、断线重连后续再补。
## Battle 状态同步补充

- 怪物状态使用 `route_id + spawn_tick + progress + speed` 表达路径进度，客户端按路线本地插值表现。
- 完整快照 `BattleSnapshotNTF` 包含玩家资源、塔、矿工和怪物列表。
- 增量 `BattleDeltaNTF` 包含资源、塔、矿工，以及怪物出生、血量变化、状态变化、死亡、到达终点、进度纠偏事件。
## Battle 结算上报

- 战斗结束由 battle 生成 `S2S_BATTLE_SETTLE_REQ` 发送给 logic。
- 结算请求包含 `room_id`、`battle_id`、胜负、开始/结束 tick、结束原因，以及每个玩家的金币、魔力、击杀数、悬赏怪召唤数和奖励金币。
- logic 收到后返回 `S2S_BATTLE_SETTLE_RSP`，用于确认是否接受本次结算。

