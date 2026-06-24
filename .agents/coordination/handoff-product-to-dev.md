# handoff-product-to-dev

## 需求背景

当前合作塔防已经完成 battle 状态同步领域层、logic 最小 match/battle 对接、battle 直连 join/op 骨架，但整体仍缺完整战斗闭环。当前目标不是继续零散补点，而是把 battle 做成“可联调的完整 P0 版本”：匹配、创房、进入战斗、局内战斗、战斗结束、战斗结算全部打通。配置目前允许先写死，不引入额外配置系统复杂度。

## 总体目标

把 battle 补成一个完整 P0 战斗闭环，形成以下完整链路：

1. 客户端发起匹配
2. logic 创建 battle 房间
3. logic 下发 battle 直连信息
4. 客户端进入 battle
5. battle 跑局内 tick / 操作 / 怪物 / 波次
6. battle 判断结束条件
7. battle 发送结算到 logic
8. logic 返回结算确认

## 目标范围

### A. 匹配与创房

1. logic `MATCH_JOIN_REQ` 继续作为客户端匹配入口。
2. 当前版本仍允许采用“单人立即建房、结构上兼容未来 2~3 人合作匹配”的 P0 方案。
3. logic 创建 battle 房间时，必须向 battle 传递以下最小建房上下文：
   - `room_id`
   - `player_ids`
   - `tower_deck`
   - `combat_type`
   - `level_id`
4. battle 建房成功后，logic 返回给客户端：
   - `room_id`
   - `battle_addr`
   - `battle_token`
   - `player_ids`

### B. 进入战斗

1. 客户端直连 battle，发送 `BATTLE_JOIN_REQ`。
2. battle 校验：
   - 房间存在
   - `room_id` 匹配
   - 玩家属于该房间
   - `battle_token` 合法
3. P0 token 允许先使用 battle 本地可校验的占位方案，但接口设计必须方便后续切换为正式签名 token。
4. 进入成功后，battle 返回：
   - `S2CBattleJoinRSP`
   - 当前完整 `snapshot`
5. 进入后，battle 必须记录玩家 join 状态/session，用于后续 op 校验与广播。

### C. 局内战斗

1. battle 继续采用服务端权威状态同步。
2. P0 局内支持操作：
   - 建塔
   - 魔力重随
   - 合成
   - 购买矿工
3. battle 需要有最小 tick 推进能力。
4. 操作成功后：
   - 返回 `BATTLE_OP_RSP`
   - 广播 `BATTLE_DELTA_NTF`
5. battle 必须支持低频或定时完整快照能力，为断线重连和纠偏做准备。

### D. 怪物与战斗结束条件

1. 本期允许怪物/波次配置先写死在 battle 内部，不接配置表。
2. 需要补一个最小可跑的战斗结束条件，至少包含：
   - 胜利：所有波次/怪物处理完成
   - 失败：主基地/终点被打爆，或约定失败条件满足
   - 取消/异常结束：房间强制关闭、必要玩家退出等
3. 怪物产生、推进、死亡/到点、对战斗结果的影响，需要在 battle 内形成最小闭环，而不只是协议占位。

### E. 战斗结算

1. battle 在战斗结束时，必须真正触发 `S2S_BATTLE_SETTLE_REQ` 发给 logic。
2. logic 接收到结算后，本期 P0 至少要完成：
   - 请求合法性确认
   - 最小幂等占位（接口/结构/注释层面）
   - 返回 `S2S_BATTLE_SETTLE_RSP`
3. P0 可不做最终奖励入账，但必须把“可入账信息”完整带到 logic。

## 开发任务拆分（总负责人版）

### 任务包 P0-1：battle 房间生命周期与 tick 主循环

- 目标：让 battle 房间从“只响应 join/op”升级为“能自行推进 server tick 的运行单元”。
- 输入：battle 房间创建成功后的房间实例。
- 输出：
  - 房间具备启动/停止/推进 tick 的最小机制。
  - 可驱动矿工产出、怪物推进、波次推进、结束条件检查。
- 涉及模块/文件：
  - `src/service/battle/room_manager.go`
  - `src/service/battle/battlesvr.go`
  - 必要时新增 battle room loop 文件
  - `src/service/battle/sync/room.go`
- 依赖关系：无，作为后续怪物、结束结算的基础。
- 验收标准：
  - 房间创建后可以进入“运行中”状态。
  - 不依赖客户端 op 也能推进 `server_tick`。
  - tick 推进不会破坏现有建塔/重随/合成/矿工逻辑。
- 风险：
  - 现有 room 是偏同步调用模型，接主循环时容易出现并发访问问题。

### 任务包 P0-2：写死波次/怪物生成与推进

- 目标：补一个最小怪物波次闭环，让战斗不是静态房间。
- 输入：房间的 `level_id` / 固定写死规则。
- 输出：
  - 固定波次数据（可硬编码）
  - 怪物定时出生
  - 怪物沿路线推进
  - 怪物死亡/到终点状态更新
- 涉及模块/文件：
  - `src/service/battle/sync/room.go`
  - `src/service/battle/sync/proto_adapter.go`
  - battle 新增波次/怪物调度文件（如需要）
- 依赖关系：依赖 P0-1 的 tick 主循环。
- 验收标准：
  - 房间启动后，不需要客户端额外操作也能看到怪物状态变化。
  - 快照/增量里的怪物字段能反映出生、推进、到点/死亡。
- 风险：
  - 当前怪物协议已定义，但领域层真实推进逻辑仍较薄。

### 任务包 P0-3：battle 结束条件与结束状态固化

- 目标：定义并接通最小结束条件，使 battle 能从“运行中”进入“已结束”。
- 输入：怪物波次推进结果、主基地/终点状态、房间异常状态。
- 输出：
  - `win / lose / timeout / abort` 等最小结束态
  - 房间结束后停止继续推进
- 涉及模块/文件：
  - `src/service/battle/sync/room.go`
  - `src/service/battle/battlesvr.go`
  - battle room loop 相关文件
- 依赖关系：依赖 P0-2 怪物推进闭环。
- 验收标准：
  - 所有怪物/波次处理完成后，房间可进入胜利结束态。
  - 怪物到终点或失败条件满足后，房间可进入失败结束态。
  - 房间结束后不会继续重复结算。
- 风险：
  - 如果结束状态没有原子控制，可能出现重复触发结算。

### 任务包 P0-4：battle 进房/操作/广播链路补强

- 目标：把已存在的 join/op 骨架补强到可联调状态。
- 输入：客户端 `BATTLE_JOIN_REQ`、`BATTLE_OP_REQ`。
- 输出：
  - 进房成功后稳定返回快照
  - 操作成功后稳定返回 rsp + 广播 delta
  - 对未 join / session 不匹配 / 错误 token 请求给出明确失败
- 涉及模块/文件：
  - `src/service/battle/handler.go`
  - `src/service/battle/battlesvr.go`
  - `src/service/battle/room_manager.go`
- 依赖关系：可与 P0-1 并行，但最终要适配运行中房间。
- 验收标准：
  - battle join 后能拿到完整快照。
  - op 成功后同房玩家都能收到 delta。
  - battle token/session 验证失败时返回明确失败。
- 风险：
  - 当前 token 仅是占位方案，联调中要避免误以为已达正式安全要求。

### 任务包 P0-5：battle 结束触发结算上报

- 目标：把现有 `settleRoom(...)` 从 helper 变成真实结束流程的一部分。
- 输入：房间结束事件、房间当前结算数据。
- 输出：
  - battle 在结束时发送 `S2S_BATTLE_SETTLE_REQ`
  - 只发送一次
  - battle 能接收 logic 回包
- 涉及模块/文件：
  - `src/service/battle/battlesvr.go`
  - battle room loop/结束流程文件
  - `src/service/battle/sync/settlement.go`
- 依赖关系：依赖 P0-3 结束状态。
- 验收标准：
  - 房间一旦结束，会真实触发结算 RPC。
  - battle 能收到并解析 logic 的 `S2S_BATTLE_SETTLE_RSP`。
- 风险：
  - 若未处理好结束幂等，可能重复发送结算。

### 任务包 P0-6：logic 结算接收补强

- 目标：把 logic 当前“仅日志确认”的结算接收升级成可作为后续发奖/落库入口的稳定节点。
- 输入：battle 发送的 `S2S_BATTLE_SETTLE_REQ`。
- 输出：
  - 基础合法性校验
  - 最小幂等占位
  - 明确确认响应
  - 后续发奖入口预留
- 涉及模块/文件：
  - `src/service/logic/module/battle/handler.go`
  - 如必要，新增结算占位结构文件
- 依赖关系：依赖 P0-5 battle 真实发送结算。
- 验收标准：
  - logic 对同一结算请求具备最小防重复入口设计。
  - logic 返回 `S2S_BATTLE_SETTLE_RSP` 语义明确。
- 风险：
  - 本期不做完整持久化，幂等只能做到占位级别，需明确边界。

### 任务包 P0-7：battle 闭环联调与文档修正

- 目标：把 battle P0 链路固化成可复现的联调路径，并修正文档与运行说明。
- 输入：前 6 个任务包完成后的实现。
- 输出：
  - 最小联调步骤
  - 关键日志观察点
  - README / `.agents/server.md` / 协作文档同步
- 涉及模块/文件：
  - `README.md`
  - `.agents/server.md`
  - `.agents/coordination/*.md`
  - 如需，`conf/*.yaml`
- 依赖关系：依赖前面任务包完成。
- 验收标准：
  - 新人看文档能知道 battle P0 链路怎么验证。
  - 文档不再继续引用明显过时的 `conf/local.yaml` / `cmd/server` 事实。
- 风险：
  - 若文档不修，后续所有角色会继续被旧信息误导。

## 建议开发顺序

1. P0-1 battle 房间生命周期与 tick 主循环
2. P0-2 写死波次/怪物生成与推进
3. P0-3 battle 结束条件与结束状态固化
4. P0-4 battle 进房/操作/广播链路补强
5. P0-5 battle 结束触发结算上报
6. P0-6 logic 结算接收补强
7. P0-7 battle 闭环联调与文档修正

> 说明：P0-4 可与 P0-1 部分并行，但最终要适配房间运行态，不建议完全独立收尾。

## 不做范围

1. 正式多人匹配排队策略优化。
2. battle token 正式安全体系。
3. 完整断线重连恢复体验。
4. 塔属性、技能、buff、完整 combat 数值系统。
5. 配置表化。
6. 完整结算落库与发奖。

## 涉及协议

### 客户端协议

继续使用当前 battle 协议：
- `BATTLE_JOIN_REQ / RSP`
- `BATTLE_OP_REQ / RSP`
- `BATTLE_SNAPSHOT_NTF`
- `BATTLE_DELTA_NTF`

如确有必要可补字段，优先考虑：
- `battle_id`
- `server_tick`
- 波次/阶段信息
- 战斗结束事件

### 服务间协议

继续使用：
- `S2S_BATTLE_CREATE_ROOM_REQ / RSP`
- `S2S_BATTLE_SETTLE_REQ / RSP`

## 涉及配置/数据

1. 本期战斗参数允许先写死在 battle 内。
2. 代码组织要保留后续配置化边界。
3. 暂不强制 Mongo/Redis 写 battle 局内状态。
4. logic 结算接收处如需做最小记录，可先日志/内存占位。

## 验收标准

### 1. 匹配/创房
- 输入：客户端发送 `MATCH_JOIN_REQ`
- 期望：logic 返回 `room_id`、`battle_addr`、`battle_token`、`player_ids`
- 期望：battle 内成功创建对应房间

### 2. 进入战斗
- 输入：客户端使用正确 `room_id + battle_token` 发送 `BATTLE_JOIN_REQ`
- 期望：battle 返回成功 `BATTLE_JOIN_RSP`
- 期望：响应内携带当前完整快照
- 输入：错误 token 或不属于该房间的玩家进入
- 期望：明确失败，不允许进房

### 3. 局内操作
- 输入：已 join 玩家发送 build/reroll/merge/buy miner
- 期望：battle 做权威校验
- 期望：成功时返回 `BATTLE_OP_RSP`
- 期望：同房间已 join 玩家收到 `BATTLE_DELTA_NTF`
- 输入：未 join 或 session 不匹配玩家发送 op
- 期望：明确失败

### 4. Tick 与怪物/结束条件
- 期望：房间存在自动推进 tick 的最小机制
- 期望：怪物/波次可推进到战斗结束
- 期望：胜利/失败/异常结束至少能区分一种以上结束原因

### 5. 结算
- 输入：战斗达到结束条件
- 期望：battle 真实发送 `S2S_BATTLE_SETTLE_REQ` 给 logic
- 期望：logic 返回 `S2S_BATTLE_SETTLE_RSP`
- 期望：battle 能收到结算确认

## 风险/阻塞

1. README 与代码事实不一致，开发不要继续依赖旧 README 启动/配置说明。
2. `go test ./...` 不是当前可靠验收标准；优先以 battle/logic 目标包测试为准。
3. battle 进入联调后，最容易暴露的问题是：
   - token/session 设计过弱
   - tick 未真正驱动
   - 结束条件只停留在 helper，没接主循环
4. 本期是 P0 闭环，开发必须优先“打通链路”，不要过早扩展复杂数值系统。
