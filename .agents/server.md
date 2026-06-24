# heli 服务器项目文档

> 项目路径：`E:\work\heli\server`  
> Go module：`game`  
> 当前定位：Go 游戏服务端工程，已从最小 TCP 联机样例演进为多服务/多依赖基础库工程。  
> 最近同步：2026-06-24（基于当前仓库文件事实更新）。

---

## 1. 当前项目事实

当前仓库已经不再是单一的最小 TCP 样例骨架，现状更接近“通用服务基础设施 + auth/gate/battle/logic 等服务拆分中的游戏服务端工程”：

- `go.mod` 仍为模块 `game`，Go 版本 `1.26.4`。
- 配置目录当前存在 `conf/auth.yaml`、`conf/gate.yaml`、`conf/global.yaml`、`conf/logic.yaml`，原 `conf/local.yaml` 已不存在。
- 当前仓库根目录未发现 `cmd/`，也未发现 `src/main.go`。
- 已存在服务目录：`src/service/auth/`、`src/service/gate/`、`src/service/battle/`。
- 已新增较多基础组件：`deps/etcd`、`deps/redis`、`deps/router`、`deps/server`、`deps/service_mgr`、`deps/transport`、`deps/xgrpc`、`deps/xhttp` 等。
- battle 方向当前已落地 `src/service/battle/sync/` 作为局内权威状态同步领域层。
- proto 生成脚本当前存在 `tools/gen_proto.ps1` 与 `tools/server_gen_proto.py`；README 中写的 `tools/gen-proto.ps1` 与当前文件名不一致。

---

## 2. 当前应作为“项目记忆”的重点

### 2.1 硬约束

- proto 调用统一使用 `google.golang.org/protobuf/proto`。
- 不手工修改 `.pb.go` 表达业务语义；业务语义回到 `.proto` 或业务代码。
- 修改 proto 时仅编辑 `tools/proto/*.proto`，再通过脚本生成代码。

### 2.2 当前协作主线

截至 2026-06-24，最近明确完成的开发主线是：

- 目标：编写 battle 战斗服状态同步领域层。
- 范围：`src/service/battle/sync` 的内存权威状态、快照、增量事件，以及建塔/重随/合成/矿工操作。
- 不做范围：本次未接网络、未改 proto、未生成 pb.go、未实现杀怪/波次/悬赏怪物金币来源。

### 2.3 battle/sync 当前能力

已实现文件：

- `src/service/battle/sync/room.go`
- `src/service/battle/sync/room_test.go`

已记录能力：

- `RoomConfig`：初始资源、建塔/重随/矿工成本、矿工产出、塔等级上限、玩家建造范围、默认塔池。
- `Room`：维护玩家资源、塔、矿工、`server_tick`、增量事件。
- `BuildTower`：校验玩家、建造范围、格子占用、金币；扣金币并生成 1 级随机塔。
- `RerollTower`：仅允许重随自己的塔；扣魔力；等级不变，只随机塔种类。
- `MergeTower`：要求同玩家、同类型、同等级、未达等级上限；新塔保留主塔位置，材料塔位置清空。
- `BuyMiner` / `AdvanceToTick`：购买矿工并按固定 tick 产出魔力。
- `Snapshot`：输出玩家、塔、矿工快照。
- `FlushDeltas`：输出并清空资源变化、建塔、重随、合成、买矿工、矿工产出等增量事件。

---

## 3. 当前验证事实

### 3.1 已通过

```powershell
go test ./src/service/battle/sync
```

### 3.2 当前全量测试阻塞

`go test ./...` 当前未完成全量通过。已知交接中记录的阻塞项包括：

- `src/service/auth/bs.go` 引用缺失包 `game/src/backend`
- `src/service/gate/gatesvr.go` 引用缺失包 `game/src/metrics`
- `src/service/gate/module/login/handler.go` 引用缺失包 `game/src/common/bi`
- 持久化/生成代码仍存在既有缺口，历史交接中记录过 `pb.GamerMailDataNew` 等未定义问题

后续任何人声称“项目全量可编译/可启动”前，应重新以当前代码事实复核。

---

## 4. 当前文档不一致点

以下内容已确认与当前代码事实不一致，后续阅读时不要继续沿用旧说法：

1. **旧说法：** 项目默认配置为 `conf/local.yaml`  
   **当前事实：** 当前 `conf/` 下未发现 `local.yaml`，存在 `auth.yaml`、`gate.yaml`、`global.yaml`、`logic.yaml`。

2. **旧说法：** 启动命令为 `go run ./cmd/server`  
   **当前事实：** 当前仓库未发现 `cmd/` 目录；需按具体服务入口重新确认启动方式。

3. **旧说法：** `src/main.go` 为占位入口  
   **当前事实：** 当前未发现 `src/main.go`。

4. **旧说法：** proto 脚本是 `tools/gen-proto.ps1`  
   **当前事实：** 当前实际存在的是 `tools/gen_proto.ps1`。

5. **旧说法：** 项目仍主要围绕最小 TCP/Mongo/Redis MVP 目录组织  
   **当前事实：** 仓库已出现 auth/gate/battle、多类 deps 基础设施和更大规模 proto/config 产物，应按新工程事实理解。

---

## 5. 当前建议的维护策略

- 继续保留 `.agents/coordination/` 作为协作事实来源，优先相信最新交接文件而不是旧总览文档。
- 后续如有人补齐新的服务入口、配置加载链路、proto 生成目标，应同步修正 README 与本文档。
- 如要恢复“项目一键启动/全量测试通过”的记忆，必须先跑通实际命令并记录结果，不要凭旧文档推断。

---

## 6. 下次更新本文档时优先核对

1. `go.mod`
2. `conf/`
3. `src/service/`
4. `src/proto/`
5. `tools/` 下 proto 脚本
6. `.agents/coordination/status-board.md`
7. `.agents/coordination/handoff-dev-to-test.md`
