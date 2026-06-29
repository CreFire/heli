# protocol-and-gameplay-notes

## 当前项目形态备注

- 当前仓库已不再只是最小 TCP 联机样例，已演进出 `auth`、`gate`、`battle`、`logic` 等服务方向，以及较多基础设施依赖目录。
- battle 方向当前已有独立的局内状态同步领域层实现：`src/service/battle/sync/`。
- 任何后续协议、接入、启动方式讨论，都要优先以当前代码事实和最新交接文件为准，不再依赖旧的 `conf/local.yaml` / `cmd/server` 假设。

## 当前 MVP 闭环（历史基线）

1. 客户端建立 TCP/WebSocket 长连接。
2. 客户端发送登录请求，服务端创建或更新玩家信息。
3. 客户端发送匹配请求，服务端按房间人数组房。
4. 客户端发送玩家操作，服务端按房间广播快照或操作。
5. 客户端发送心跳，服务端维持连接状态。

> 注：以上仍可作为历史 MVP 闭环基线，但不等同于当前项目完整结构。

## 当前协议文件

- `tools\proto\client.proto`：客户端登录、匹配、操作、快照、心跳消息。
- `tools\proto\common.proto`：通用结构和需要持久化的结构。
- `tools\proto\micro.proto`：基础消息 ID 和服务间消息范围。
- `tools\proto\error.proto`：错误码。

## Logic 任务模块现状

- 当前任务逻辑目录：`src/service/logic/module/task/`
- 原 `taskbiz` 已并回 `task` 模块，不再作为独立目录维护。
- 当前任务模块已从旧 `TbDailyTask` 切到新版 `TbTask`。

### 当前已落地规则

- 奖励字段 `RewardItem []int32` 按 `ItemId, Num` 成对解析。
- `TaskResetType` 规则：
  - `0`：不重置
  - `1`：每日刷新
  - `2`：每周刷新
- `TaskUnlock` 当前按最小前置任务规则处理：
  - `<= 0` 直接开放
  - `> 0` 需前置任务达到 `TASK_OVER`

### 当前未落地规则

以下新版配置语义当前仍未完整接入：

- `ConditionType`
- `Key`
- `TaskActivation`
- 完整任务事件驱动进度更新
- 完整完成条件判定

因此当前任务能力应理解为：

- 已完成新表接线与结构迁移
- 已完成奖励发放解析
- 已完成日/周刷新
- 未完成完整玩法闭环

## 最近已落地的 battle/sync 玩法能力

- 局内初始资源：每个玩家拥有初始金币和初始魔力。
- 玩家塔池：每个玩家使用自己的塔防卡组作为随机塔种类池；若后续未配置卡组，可用默认塔池兜底。
- 建塔范围：玩家只能在自己的建造范围内建造塔；点击自己范围内的高地空地，花费金币，瞬间建造 1 座随机种类的 1 级塔。
- 魔力重随：玩家可对任意一座自己的已有塔花费魔力重随；重随后塔等级不变，只随机塔种类；不能重随其他玩家的塔。
- 塔合成：客户端提交 `main_tower_id` 和 `material_tower_id`；两座塔必须同属该玩家、相同种类且相同等级；合成后生成 1 座更高一级的随机种类塔，新塔出现在 `main_tower_id` 的位置，`material_tower_id` 的位置变为空地；塔等级从 1 级开始，最高 5 级；5 级塔不能继续合成。
- 矿工：没有矿位概念；玩家花费金币购买挖矿工，矿工按固定时间间隔产出固定魔力。
- 权威归属：以上局内资源、建塔、重随、合成、矿工产出、金币获取均由 battle 服务权威校验和结算，客户端只发送操作意图。

## 合作塔防同步与接入决策

- 同步模式：合作塔防战斗采用服务端权威状态同步，不采用纯帧同步。
- 客户端输入特征：输入较少，主要是准备、建塔、升级、卖塔、释放技能等低频离散操作。
- 怪物同步方向：小怪数量多且沿固定路线移动，优先使用 `route_id`、`spawn_tick`、`progress`、`speed` 等路径进度数据进行客户端表现推演；服务端同步关键事件、血量/状态增量和低频快照。
- 服务分工：auth 负责身份认证；logic 负责客户端登录入口、session、匹配、房间入口和分配 battle 连接信息；battle 负责战斗期客户端连接、房间权威战斗状态、tick 推进、怪物/塔/技能/波次/结算和状态同步消息生成。
- 战斗接入方式：战斗开始后客户端直连 battle；logic 在匹配成功或创建房间成功后下发 battle 地址、room_id 和一次性/短期战斗准入 token。
- 选择原因：状态同步下战斗广播和快照直接由 battle 下发给客户端，可减少 logic 中转链路与转发压力；战斗期消息归属更清晰，battle 可以直接管理房间内连接、断线重连和快照恢复。
- 安全要求：battle 需要校验 logic/auth 签发的战斗准入 token，确认 `player_id`、`room_id`、过期时间和签名，禁止客户端仅凭 `room_id` 进入房间。
- 后续演进：如果 battle 直连带来公网暴露或鉴权复杂度问题，可再评估专用战斗网关；默认不回退到 logic 中转。

## 当前已知测试事实

- `go test ./src/service/battle/sync` 已通过。
- `go test ./src/service/logic/module/task/...` 已通过。
- `go test ./src/service/logic/actor/...` 已通过。
- `go test ./...` 仍不能宣称全量通过；本次未做全仓联机闭环验证。

## 后续候选需求池

- task 新配置的进度驱动与完成判定补齐。
- battle sync 接入真实服务连接/房间管理。
- battle 直连鉴权与准入 token 校验。
- battle 客户端进房 handler。
- 怪物波次。
- 塔属性与升级。
- 战斗结算持久化与奖励入账。
- 断线重连。
- 房间状态恢复。
- 更完整的协议错误码。

## 记录规则

- 新需求先写清目标和不做范围，再进入 product 到 dev 的交接。
- 协议变更需要同时记录客户端期望、服务端处理和兼容影响。
- 若项目结构继续演进，优先更新本文档与 `.agents/server.md`，避免旧的启动/配置记忆误导后续协作。

## Battle 客户端直连协议草案

- `BATTLE_JOIN_REQ` / `BATTLE_JOIN_RSP`：客户端直连 battle 后，携带 `room_id` 与 `battle_token` 进入房间；battle 校验通过后返回当前快照。
- `BATTLE_OP_REQ` / `BATTLE_OP_RSP`：客户端提交局内操作，包含 `op_id` 和 `BattlePlayerOp`；当前操作类型覆盖建塔、魔力重随、塔合成、购买矿工。
- `BATTLE_SNAPSHOT_NTF`：battle 下发完整快照，包含玩家资源、矿工和塔列表。
- `BATTLE_DELTA_NTF`：battle 下发状态增量，包含资源变化、建塔、重随、合成、购买矿工、矿工产出等事件。
- proto 源位于 `tools\proto\client.proto` 和 `tools\proto\micro.proto`，生成结果位于 `src\proto\pb\client.pb.go` 和 `src\proto\pb\micro.pb.go`。

## Battle 怪物状态协议

- `BattleMonsterState` 使用路径进度同步：`monster_id`、`monster_type`、`route_id`、`spawn_tick`、`progress`、`speed`、`hp`、`max_hp`、`status`。
- `S2CBattleSnapshotNTF.monsters` 下发完整怪物列表，用于进房、断线重连和低频纠偏。
- `BattleStateDelta.monster` 下发怪物增量，类型包括出生、血量变化、状态变化、死亡、到达终点、进度纠偏。
- 怪物坐标不作为协议核心字段；客户端应根据路线配置与进度推算坐标。

## Match / Battle 对接现状

- `logic.match` 已接客户端 `MATCH_JOIN_REQ`，当前 P0 逻辑为单人立即建房，不做完整多人队列。
- `logic.match` 调用 `logic.battle.CreateRoom(...)`，再由 logic 向 battle 发起 `S2S_BATTLE_CREATE_ROOM_REQ`。
- battle 返回 `room_id`、`battle_addr`、`battle_token` 给 logic，再由 logic 回给客户端。
- 当前 `battle_token` 仅为最小占位串，尚未实现签名、过期时间和防伪校验。

## Battle 结算上报协议

- `S2S_BATTLE_SETTLE_REQ`：battle 在房间战斗结束时发送给 logic，携带房间结算结果。
- `S2S_BATTLE_SETTLE_RSP`：logic 对结算请求的确认响应。
- 结算字段包含：`room_id`、`battle_id`、`win`、`start_tick`、`end_tick`、`finish_reason`、玩家结算列表。
- 玩家结算字段包含：`player_id`、局内剩余 `gold` / `mana`、`kill_count`、`summon_bounty_count`、`reward_gold`。
- 当前代码事实：
  - battle 已有 `settleRoom(...)` 发送能力。
  - logic 已注册并接收 `S2S_BATTLE_SETTLE_REQ`，返回 `S2S_BATTLE_SETTLE_RSP`。
  - logic 目前仅做最小确认，不做幂等、落库和奖励结算。
