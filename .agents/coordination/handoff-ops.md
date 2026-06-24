# handoff-ops

## 环境目标

- 目标：整理当前 `battle + logic` 第一阶段本地联调所需的最小运行说明，供 battle P0 闭环后续开发/测试联调用。
- 范围：仅基于当前代码事实核对服务入口、配置文件、依赖、启动命令、日志观察点、已知阻塞。
- 不做：不修改业务代码，不承诺当前仓库已具备 battle/logic/gate/auth 全链路一键跑通能力。

## 检查命令

在目录 [E:\work\heli\server](`E:\work\heli\server`) 进行过以下核对：

```powershell
Get-ChildItem conf
Get-ChildItem src\service
Get-ChildItem -Recurse src\service\battle
Get-ChildItem -Recurse src\service\logic
Get-ChildItem -Recurse -Include *.go | Select-String -Pattern 'func main\('
Get-Content conf\global.yaml
Get-Content conf\logic.yaml
Get-Content src\service\battle\main.go
Get-Content src\service\battle\battlesvr.go
Get-Content src\service\battle\handler.go
Get-Content src\service\logic\main.go
Get-Content src\service\logic\logicsvr.go
Get-Content src\service\logic\module\battle\handler.go
Get-Content src\service\logic\module\matchbiz\match_module.go
Get-Content deps\server\service.go
Get-Content src\configdoc\config.go
Get-Content src\configdoc\config_global.go
Get-Content src\configdoc\config_server.go
Get-Content src\service\battle\logs\log -Tail 80
Get-Content src\service\logic\module\logs\log -Tail 80
```

## 结果

### 1. 当前 battle/logic 服务入口事实

- battle 入口存在：[E:\work\heli\server\src\service\battle\main.go](`E:\work\heli\server\src\service\battle\main.go`)
- logic 入口存在：[E:\work\heli\server\src\service\logic\main.go](`E:\work\heli\server\src\service\logic\main.go`)
- 当前服务启动统一依赖 `-f` 参数传入配置文件，来自 [E:\work\heli\server\deps\server\service.go](`E:\work\heli\server\deps\server\service.go`) 的 `pflag.String("f", "", ...)`；**不传 `-f` 会直接报 `config file path is empty`**。
- 当前仓库 `conf/` 下只有：
  - [E:\work\heli\server\conf\auth.yaml](`E:\work\heli\server\conf\auth.yaml`)
  - [E:\work\heli\server\conf\gate.yaml](`E:\work\heli\server\conf\gate.yaml`)
  - [E:\work\heli\server\conf\global.yaml](`E:\work\heli\server\conf\global.yaml`)
  - [E:\work\heli\server\conf\logic.yaml](`E:\work\heli\server\conf\logic.yaml`)
  - [E:\work\heli\server\conf\robot.yaml](`E:\work\heli\server\conf\robot.yaml`)
- **当前未发现 battle.yaml**。因此 battle 服务虽然已有 `main.go` 与业务实现，但仓库内缺少 battle 对应 YAML 配置文件，现阶段不能按“开箱即跑”的标准交给联调同学。

### 2. battle 第一阶段联调链路事实

当前 battle/logic 方向已具备的链路骨架：

1. logic 匹配模块在 [E:\work\heli\server\src\service\logic\module\matchbiz\match_module.go](`E:\work\heli\server\src\service\logic\module\matchbiz\match_module.go`) 中调用 `battlemodule.CreateRoom(...)`。
2. logic battle 模块在 [E:\work\heli\server\src\service\logic\module\battle\handler.go](`E:\work\heli\server\src\service\logic\module\battle\handler.go`) 中通过 RPC 向 battle 发送 `MSG_ID_S2S_BATTLE_CREATE_ROOM_REQ`。
3. battle 在 [E:\work\heli\server\src\service\battle\handler.go](`E:\work\heli\server\src\service\battle\handler.go`) 注册：
   - `MSG_ID_S2S_BATTLE_CREATE_ROOM_REQ`
   - `MSG_ID_BATTLE_JOIN_REQ`
   - `MSG_ID_BATTLE_OP_REQ`
4. battle 房间管理在 [E:\work\heli\server\src\service\battle\room_manager.go](`E:\work\heli\server\src\service\battle\room_manager.go`) 中已支持：建房、玩家绑定 session、推进 tick、刷增量。
5. battle 结算回传 logic 的 RPC 已在 battle/logic 双侧各自落一半：
   - battle 发 `MSG_ID_S2S_BATTLE_SETTLE_REQ`
   - logic 回 `MSG_ID_S2S_BATTLE_SETTLE_RSP`

这说明：**battle 第一阶段更适合先做 logic <-> battle 服务间联调，再看 gate/client 接入。**

### 3. 当前 battle/logic 本地最小依赖

按当前代码事实，本地联调至少需要：

- MongoDB：`mongodb://127.0.0.1:27017`
- Redis：`redis://127.0.0.1:6379`
- etcd：`etcd://127.0.0.1:1003`
- 配表目录：`conf/global.yaml` 中当前 `gameDataPath: "../docconf"`

说明：

- logic、battle 都走 `server.MS.Start()`，默认会初始化 Mongo、Redis。
- 除 `robot` 或显式 `notEtcd: true` 的服务外，默认还会连 etcd；当前 `logic.yaml` 未配置 `notEtcd`，battle 配置文件也尚不存在，因此 **battle/logic 联调默认应视为需要 etcd**。
- `conf/global.yaml` 当前写的是 `etcd://127.0.0.1:1003`，不是常见的 `2379`，联调前必须先确认本机 etcd 实际监听端口是否就是 `1003`。

### 4. 当前可直接使用的启动命令事实

#### 4.1 logic

当前 logic 启动命令应为：

```powershell
go run ./src/service/logic -f ./conf/logic.yaml
```

若从 Windows PowerShell 执行，建议工作目录固定为 [E:\work\heli\server](`E:\work\heli\server`)。

#### 4.2 battle

battle 入口命令形式应为：

```powershell
go run ./src/service/battle -f <battle配置文件路径>
```

但因为仓库内**没有现成 `conf/battle.yaml`**，所以目前只能给出“命令形态”，不能给出现成可执行的 battle 启动命令。

### 5. battle 后续联调建议的最小配置要求

需要由 dev/ops 后续补齐一个 battle 配置文件，至少覆盖以下字段（字段名以 [E:\work\heli\server\src\configdoc\config_server.go](`E:\work\heli\server\src\configdoc\config_server.go`) 为准）：

```yaml
id: <非冲突实例ID>
type: "battle"
cluster: "dev01"
ip: "127.0.0.1"
port: <battle监听端口>
log:
  level: "info"
  path: "./logs"
  fileName: "battle"
  rotation: "daily"
  maxFileSizeMB: 100
  retentionDays: 14
asyncPoolSize: 512
aSyncQueueSize: 8192
# 如仅做本地脱离服务发现自测，可评估 notEtcd: true；
# 但 battle 与 logic 的 RPC/发现链路联调阶段，不建议先关 etcd。
```

建议 battle 本地端口不要与现有服务冲突，优先避开：

- auth：`5000`
- logic：`5003`
- gate：`10301`
- robot：`10401`

### 6. 联调顺序建议（按 battle P0 第一阶段）

建议按以下顺序联调，而不是一上来追求四服全开：

1. **先确认依赖存活**：Mongo / Redis / etcd。
2. **先起 logic**：确认 logic 能完成配置加载、表加载、服务注册、监听启动。
3. **再起 battle**：确认 battle 能注册到同 cluster，并能被 logic 发现。
4. **先做服务间联调**：重点验证 `logic -> battle create room` 与 `battle -> logic settle`。
5. **最后再接 gate / client / robot**：因为当前第一阶段 battle 关注的是房间创建、进房、局内 op、tick、结算回传，不必一开始就把完整客户端链路绑死。

## 配置变更

- 本次未修改任何业务配置文件。
- 仅整理运行说明，明确指出当前缺少 [E:\work\heli\server\conf\battle.yaml](`E:\work\heli\server\conf\battle.yaml`) 是 battle 本地联调前置缺口。

## 日志摘要

### 重点观察点

#### logic 启动后重点看

- 是否出现 `server start ok!`
- 是否出现 `mongo connect ok!`
- 是否出现 `redis connect ok!`
- 是否出现 logic 网络监听启动日志
- 周期性 `print_status`：`[run-status] ... gamerOnline=...`
- 若 battle 已启动，关注：
  - `get battle load info failed` 是否持续报错
  - `prewarm grpc conn failed` 是否持续报错

#### battle 启动后重点看

- 是否出现 `server start ok!`
- 是否出现 `mongo connect ok!`
- 是否出现 `redis connect ok!`
- 周期性 `print_status`：`[run-status] ... roomCount=...`
- 创建房间时关注：
  - `battle create room failed` 是否出现
- 结算回传时关注：
  - `pick logic server failed`
  - `battle settle rsp decode failed`

### 当前已观察到的日志风险

在现有日志文件中已能看到大量 `msg/parser_pb.go:52 ... bind msg err: proto: not found`，涉及：

- `BATTLE_JOIN_REQ`
- `BATTLE_OP_REQ`
- `BATTLE_SNAPSHOT_NTF`
- `BATTLE_DELTA_NTF`
- 以及若干 match/item 相关消息

对应日志位置示例：

- [E:\work\heli\server\src\service\battle\logs\log](`E:\work\heli\server\src\service\battle\logs\log`)
- [E:\work\heli\server\src\service\logic\module\logs\log](`E:\work\heli\server\src\service\logic\module\logs\log`)

这说明当前 battle/logic 虽有协议 ID 与 handler 骨架，但**本地消息绑定/解析层仍有 proto 注册缺口**。这属于联调前必须显式关注的非环境类风险点。

## 阻塞/建议

### 当前阻塞

1. 仓库内缺少 battle 独立配置文件，battle 服务无法按统一方式直接启动。
2. `conf/global.yaml` 使用 `etcd://127.0.0.1:1003`，本机若没有对应端口的 etcd，会在 battle/logic 启动阶段直接失败。
3. 现有 battle / logic 日志已暴露多处 `proto: not found`，说明 battle 第一阶段即便服务拉起，也未必能顺利完成 join/op/snapshot/delta 的消息解析联调。

### 最小联调建议

1. **先补 battle.yaml，再谈 battle 实服联调。**
2. **先把 etcd 端口真相确认掉**：若本地不是 `1003`，应统一修改联调使用的配置，而不是靠口头约定。
3. **优先验证 logic+battle 双服链路**：
   - logic 发 `S2S_BATTLE_CREATE_ROOM_REQ`
   - battle 返回 `battle_addr / room_id / battle_token`
   - battle 结束后回 `S2S_BATTLE_SETTLE_REQ`
4. **将 `proto: not found` 视为 battle 第一阶段的高优先级联调阻塞**，需要 dev 在协议注册/生成链路上先补齐，否则后续 gate/client/robot 接入会放大问题。
5. gate / auth / robot 暂不作为本次 ops 最小说明的启动前提；battle P0 第一阶段先围绕 `logic <-> battle` 收敛问题更划算。
