# heli cooperative tower-defense server

当前项目已从最小 TCP 联机样例演进为 **多服务拆分的 Go 游戏服务端工程**，包含 `auth`、`gate`、`logic`、`battle`、`robot` 等服务方向，并配套 MongoDB、Redis、Etcd、protobuf、配置文档与生成工具链。

## 当前项目配置概览

配置解析使用 Viper，但当前仓库配置结构已经不是旧版的 `conf/local.yaml` 单文件模式，而是 **全局配置 + 按服务拆分配置**。

当前 `conf/` 目录包含：

```text
conf/global.yaml
conf/auth.yaml
conf/gate.yaml
conf/logic.yaml
conf/robot.yaml
```

### 配置职责划分

#### `conf/global.yaml`

全局公共配置，主要包含：

- MongoDB 连接：
  - `mongo.dsn`
  - `mongo.dbName`
- Redis 连接：
  - `redisDsn`
- Etcd 连接：
  - `etcdDsn`
- 游戏配置表路径：
  - `gameDataPath`
- 对外网关信息：
  - `gateExtAddr`
  - `gatePublicHost`
- 环境和调试开关：
  - `gameArea`
  - `enableLockCheck`
  - `isdebug`
- 全局日志配置：
  - `log.level`
  - `log.rotation`
  - `log.retentionDays`
  - `log.fileOut`
  - `log.stdOut`

当前默认依赖地址：

- MongoDB: `mongodb://127.0.0.1:27017`
- Redis: `redis://127.0.0.1:6379`
- Etcd: `etcd://127.0.0.1:1003`

#### `conf/auth.yaml`

认证服务配置，主要负责：

- 服务实例信息：
  - `id`
  - `type: "auth"`
  - `cluster`
- 服务监听地址：
  - `ip`
  - `port`
- 登录限流与排队参数：
  - `auth.rate_per_ip`
  - `auth.rate_per_gate`
  - `auth.rate_max`
  - `auth.gate_queue_size`
- 第三方登录参数：
  - `auth.appid`
  - `auth.sc`

#### `conf/gate.yaml`

网关服务配置，主要负责：

- 服务实例信息：
  - `id`
  - `type: "gate"`
  - `cluster`
- 服务监听地址：
  - `ip`
  - `port`
- 网络接入方式：
  - `net.transport`
  - `net.wsPath`
- 并发处理参数：
  - `asyncPoolSize`
  - `aSyncQueueSize`

当前 gate 默认使用：

- `websocket`
- 路径：`/ws`

#### `conf/logic.yaml`

逻辑服务配置，主要负责：

- 服务实例信息：
  - `id`
  - `type: "logic"`
  - `cluster`
- 服务监听地址：
  - `ip`
  - `port`
- 并发处理参数：
  - `asyncPoolSize`
  - `aSyncQueueSize`
- 日志输出参数

#### `conf/robot.yaml`

机器人/自动化测试服务配置，主要负责：

- 服务实例信息：
  - `id`
  - `type: "robot"`
  - `cluster`
- 服务监听地址：
  - `ip`
  - `port`
- 机器人使用的数据路径：
  - `gameDataPath`
- 网络接入方式：
  - `net.transport`
  - `net.wsPath`
- 机器人任务参数：
  - `robot.name`
  - `robot.count`
  - `robot.auth`
  - `robot.loginRate`
  - `robot.actionIntervalMs`
  - `robot.recentReqLimit`

## 当前任务配置介绍

如果从“任务配置”角度理解本项目，可以分成三层：

### 1. 全局环境配置

由 `conf/global.yaml` 控制：

- 数据库与中间件连接
- 游戏配置表路径
- 运行环境标识
- 全局日志行为

### 2. 服务实例配置

由 `conf/auth.yaml`、`conf/gate.yaml`、`conf/logic.yaml`、`conf/robot.yaml` 控制：

- 服务类型
- 服务实例 ID
- 集群名称
- 监听地址和端口
- 日志路径与日志级别
- 异步池与队列大小

### 3. 业务任务参数

具体按服务划分：

- `auth.yaml`
  - 登录限流
  - 排队阈值
  - 第三方登录参数
- `gate.yaml`
  - WebSocket 接入参数
  - 网关并发处理能力
- `logic.yaml`
  - 核心逻辑服务实例参数
- `robot.yaml`
  - 机器人数量
  - 登录速率
  - 行为间隔
  - 最近请求限制

其中最接近“可执行任务参数”的是 `robot.yaml`，适合用于：

- 自动化冒烟测试
- 机器人联调
- 登录压测
- 简单行为回放

## 任务模块现状

当前任务逻辑位于：

- `E:\work\heli\server\src\service\logic\module\task`

此前独立的 `taskbiz` 已合并回 `task` 模块，当前对外入口与逻辑实现都统一放在该目录下。

### 当前任务配置来源

任务模块现在读取新版配置表：

- `TbTask`
- 配置结构：`docpb.Task`

不再使用旧的：

- `TbDailyTask`
- `DailyTask` 旧字段族（如 `RewardPoints`、`CompleteValue`、`KeyId`、`IsOpen`）

### 当前已接入的新任务字段

任务模块当前已接入并使用：

- `TaskID`
- `TaskType`
- `RewardItem`
- `TaskUnlock`
- `TaskResetType`

### RewardItem 解析规则

`RewardItem []int32` 当前按相邻两位一组解析：

- 奇数位：`ItemId`
- 偶数位：`数量`

例如：

```text
[1001, 10, 1002, 3]
```

表示：

- `1001 x 10`
- `1002 x 3`

如果数组长度不是偶数，或存在非正数配置，当前奖励发放会直接按失败处理。

### TaskResetType 规则

当前任务刷新规则：

- `0`：不重置
- `1`：每日刷新
- `2`：每周刷新

当前实现方式：

- 刷新任务列表时，先检查已有任务是否跨刷新周期
- 已过期任务会被删除
- 然后再按 `TbTask` 配置重新补开

### 当前任务开放规则

当前 `TaskUnlock` 先按最小规则处理：

- `TaskUnlock <= 0`：直接开放
- `TaskUnlock > 0`：要求前置任务状态为 `TASK_OVER`

### 当前未完成部分

新版任务配置还有一部分语义尚未完全接入，当前代码未明确落地：

- `ConditionType`
- `Key`
- `TaskActivation`
- 更细粒度的任务进度推进与完成判定

因此目前任务模块已完成的是：

- 新配置切换
- 奖励解析
- 日/周刷新
- 模块合并

但“任务事件驱动进度更新”仍需要后续继续补齐。

## 配置相关注意事项

1. 旧文档中的 `conf/local.yaml` 当前已不再是仓库现状，不应继续作为默认配置入口理解。
2. `global.yaml` 中的 `gameDataPath` 当前为 `../docconf`，而 `robot.yaml` 中的 `gameDataPath` 为 `../tools/excel`，两者用途不同，修改时需要区分。
3. 修改配置时，除更新 `conf/` 文件外，还应同步检查：
   - `src/configdoc/`
   - `.agents/server.md`
   - 本 README

## 当前启动依赖

- MongoDB: `127.0.0.1:27017`
- Redis: `127.0.0.1:6379`
- Etcd: `127.0.0.1:1003`

## Docker Compose

仓库已提供：

```text
docker-compose.yml
```

当前 compose 仅包含基础依赖：

- `mongo`
- `redis`
- `etcd`

### 仅启动基础依赖

```powershell
docker compose up -d
```

### 查看日志

```powershell
docker compose logs -f
```

查看某个服务：

```powershell
docker compose logs -f gate
```

### 停止并删除容器

```powershell
docker compose down
```

### 说明

- 当前 compose 只负责拉起本地开发依赖，不负责启动 `auth/gate/logic/robot` 应用服务。
- etcd 容器内部标准客户端端口为 `2379`，compose 已将主机 `1003` 映射到容器 `2379`，以兼容当前 `conf/global.yaml`。

## 运行说明

当前仓库不是旧版单入口 `cmd/server` 结构；README 旧说法已不适用。  
目前应按具体服务入口分别确认运行方式，例如：

- `src/service/auth/main.go`
- `src/service/gate/main.go`
- `src/service/logic/main.go`
- `src/service/battle/main.go`
- `src/service/robot/main.go`

在声称“项目可直接启动”前，应先按当前代码事实实际验证。

## 生成 Proto

本项目使用 `tools/bin` 内置的 `protoc` 和相关插件。

当前实际 proto 脚本文件名为：

```powershell
powershell -ExecutionPolicy Bypass -File tools\gen_proto.ps1
```

也可结合：

```powershell
python tools\server_gen_proto.py
```

proto 源文件位于：

```text
tools/proto/
```

生成结果主要位于：

```text
src/proto/
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
