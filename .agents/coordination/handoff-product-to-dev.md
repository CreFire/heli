# handoff-product-to-dev

## 需求背景

当前 battle P0 第一阶段已经具备房间 join/op 基础骨架、局内基础状态同步能力，以及 battle -> logic 结算协议骨架，但第二阶段仍未补齐可稳定联调的战斗闭环关键节点。本次 product 只补 battle P0 第二阶段需求拆分，聚焦以下 3 个目标：

- P0-4：join/op/广播补强
- P0-5：结束触发结算上报
- P0-6：logic 结算接收补强

本次不重新展开匹配建房、tick 主循环、怪物波次、复杂战斗数值，只为 dev 提供第二阶段可执行交接，确保 battle 从“有骨架”推进到“关键链路可联调、可验收”。

## 本次目标范围

仅补 battle P0 第二阶段以下三个目标：

1. P0-4 battle 进房 / 操作 / 广播链路补强
2. P0-5 battle 结束触发结算上报
3. P0-6 logic 结算接收补强

> 说明：若 dev 在实现中发现 P0-4 ~ P0-6 依赖前置 battle 结束态或房间运行态，应只做必要衔接，不把任务扩成 P0-1 ~ P0-3 的重新设计。

---

## P0-4：battle 进房 / 操作 / 广播链路补强

### 目标范围

围绕 battle 现有 `BATTLE_JOIN_REQ`、`BATTLE_OP_REQ` 骨架，补到“可稳定联调”的最小可执行状态：

1. 玩家使用正确 `room_id + battle_token` 进入 battle 时，服务端必须返回明确成功响应。
2. join 成功响应中必须带当前完整快照，保证客户端进入后能立即落地当前权威状态。
3. battle 需要在房间内记录最小 join 结果：至少包括玩家已 join 状态、绑定 session，供后续 op 校验和广播使用。
4. 已 join 且 session 合法的玩家发送 battle op 时，服务端必须返回明确 `BATTLE_OP_RSP`。
5. op 成功后，battle 必须向同房间已 join 玩家广播对应 `BATTLE_DELTA_NTF`。
6. 对非法 room、非法 token、未 join、session 不匹配、op 参数不合法等请求，battle 必须返回明确失败，而不是静默丢弃。
7. 如 battle 当前已具备完整快照通知或低频全量同步能力，本次需保证 join 初始快照与后续 delta 语义一致；如尚未补齐，仅要求本次不要破坏后续快照补发扩展点。

### 不做范围

1. 不做正式 battle token 安全签名体系。
2. 不做完整断线重连恢复流程。
3. 不做观战、旁听、掉线后自动补快照等衍生能力。
4. 不新增复杂 battle 操作类型；仅围绕当前已存在的 build / reroll / merge / buy miner。
5. 不要求本次解决所有多人时序一致性优化，只要求最小广播链路可靠可验收。

### 涉及协议

客户端协议继续基于现有消息：

- `BATTLE_JOIN_REQ / BATTLE_JOIN_RSP`
- `BATTLE_OP_REQ / BATTLE_OP_RSP`
- `BATTLE_DELTA_NTF`
- 如当前已有 `BATTLE_SNAPSHOT_NTF`，其字段语义需与 join rsp 中 snapshot 对齐

客户端期望：

1. join 成功后，立即拿到可渲染当前局面的完整 snapshot。
2. op 成功后，先拿到本次 op 的 rsp，再收到房间 delta（允许同 tick 内连续到达，但语义需清晰）。
3. op 失败时，客户端能根据错误码/错误信息判断是否需要重进房、重发或停止操作。

兼容性影响：

- 优先复用现有字段。
- 如确需补字段，保持旧字段语义不变，避免破坏当前 join/op 基础调用。

### 验收标准

1. **合法进房成功**
   - 输入：已由 logic 下发正确 `room_id`、`battle_token` 的玩家发送 `BATTLE_JOIN_REQ`
   - 期望：battle 返回成功 `BATTLE_JOIN_RSP`
   - 期望：rsp 中包含当前玩家信息与完整 snapshot
   - 期望：服务端记录该玩家已 join 且绑定当前 session

2. **非法进房失败**
   - 输入：错误 `room_id`、错误 `battle_token`、非房间成员玩家任一条件下发送 `BATTLE_JOIN_REQ`
   - 期望：battle 返回明确失败
   - 期望：房间 join 记录不被错误写入

3. **合法操作成功并广播**
   - 前置：房间内至少 1 名玩家已成功 join
   - 输入：该玩家发送 build / reroll / merge / buy miner 中任一合法 `BATTLE_OP_REQ`
   - 期望：battle 返回成功 `BATTLE_OP_RSP`
   - 期望：同房间所有已 join 玩家收到对应 `BATTLE_DELTA_NTF`
   - 期望：rsp 与 delta 的 room_id、op_id、server_tick 语义一致

4. **未 join / session 不匹配操作失败**
   - 输入：未 join 玩家或 join 后 session 已失配的连接发送 `BATTLE_OP_REQ`
   - 期望：battle 返回明确失败
   - 期望：不广播伪造 delta

### 风险

1. 当前 token 仍是 P0 占位设计，容易在联调中被误解为正式安全方案。
2. 如果 join 状态、session 绑定和广播目标三者未统一，容易出现“op 成功但没人收到广播”或“未 join 连接也收到广播”。
3. 如果 rsp 与 delta 使用的 `server_tick`、`op_id` 语义不一致，客户端难以做本地确认和纠偏。

---

## P0-5：battle 结束触发结算上报

### 目标范围

围绕 battle 当前已存在的结算协议骨架与 `settleRoom(...)` 能力，补成真实结束流程的一部分：

1. 当房间满足既定结束条件并进入结束态后，battle 必须真实触发 `S2S_BATTLE_SETTLE_REQ` 发往 logic。
2. battle 必须保证同一房间同一局结算只上报一次，不允许重复发送。
3. 结算请求中至少要带上当前 README 已明确的 P0 结算关键信息：
   - `room_id`
   - `battle_id`
   - 胜负结果
   - `start_tick / end_tick`
   - 结束原因
   - 每个玩家的最小结算信息（如金币、魔力、击杀、悬赏怪召唤数、奖励金币）
4. battle 发送结算后，必须接收并解析 logic 返回的 `S2S_BATTLE_SETTLE_RSP`，作为“logic 已接收”的确认。
5. 若 logic 返回失败或链路异常，本次只要求 battle 有明确日志/状态可观测，不要求补完整重试机制。

### 不做范围

1. 不新增复杂结算奖励公式。
2. 不做最终奖励入库、发奖、邮件、背包入账。
3. 不做完整补偿重试队列、消息持久化队列。
4. 不扩展到赛季积分、任务进度、成就等战斗外系统。
5. 不重新定义 battle 结束条件本身；本次只承接“结束后如何上报结算”。

### 涉及协议

服务间协议继续使用：

- `S2S_BATTLE_SETTLE_REQ`
- `S2S_BATTLE_SETTLE_RSP`

服务端行为期望：

1. 结算上报由 battle 主动发起，而不是仅停留在 helper 或单元测试层。
2. logic 回包后，battle 能区分“已确认接收”和“未确认接收”。
3. 若 battle 结束后重复进入结算逻辑，应命中同局只上报一次的防重约束。

兼容性影响：

- 优先沿用现有 settle proto 结构。
- 如需补字段，必须仍保持 battle -> logic 最小兼容，不让现有测试骨架失效。

### 验收标准

1. **战斗结束触发上报**
   - 前置：房间进入结束态
   - 期望：battle 真实发送 `S2S_BATTLE_SETTLE_REQ` 给 logic
   - 期望：请求中包含 room / battle / result / tick / finish reason / players 等最小必要字段

2. **同一局只上报一次**
   - 输入：同一房间结束逻辑被重复触发，或结束检查被重复命中
   - 期望：battle 对外只发送一次结算请求
   - 期望：重复路径仅记录防重结果，不再次发送

3. **battle 接收结算确认**
   - 前置：logic 正常回 `S2S_BATTLE_SETTLE_RSP`
   - 期望：battle 能成功解析回包
   - 期望：日志或状态中可确认本次结算已被 logic 接收

### 风险

1. 若结束态与结算上报之间没有统一幂等控制，最容易出现重复结算请求。
2. 如果 battle 只发请求不消费回包，联调时无法确认 logic 是否真的接收成功。
3. 目前不做重试队列，logic 临时不可用时会暴露“结束了但未被确认”的空档，需通过文档明确这是 P0 边界。

---

## P0-6：logic 结算接收补强

### 目标范围

围绕 logic 当前 battle 结算接收 handler，把“仅日志确认”补成 battle 第二阶段可依赖的稳定接收节点：

1. logic 收到 `S2S_BATTLE_SETTLE_REQ` 后，先做最小合法性校验。
2. 合法性校验至少覆盖：
   - 请求体非空
   - `room_id`、`battle_id` 等关键标识非空/非默认值
   - 玩家结算列表具备最小可读性
3. logic 需要提供最小幂等占位：即便本期不做完整持久化，也要有“同一 room_id / battle_id 重复请求如何识别”的明确入口。
4. logic 需明确返回 `S2S_BATTLE_SETTLE_RSP` 成功或失败语义，供 battle 判断是否已接收。
5. logic 需要为后续奖励入账/落库预留清晰扩展位置，但本次不实现完整发奖。

### 不做范围

1. 不做完整结算持久化方案。
2. 不做正式发奖、背包变更、邮件补偿。
3. 不做跨服幂等一致性方案。
4. 不做 battle 外围系统联动，如任务、成就、排行。
5. 不要求本次落 Mongo/Redis，只要求接收语义与幂等入口明确。

### 涉及协议

服务间协议继续使用：

- `S2S_BATTLE_SETTLE_REQ`
- `S2S_BATTLE_SETTLE_RSP`

logic 期望行为：

1. 合法请求返回成功确认，battle 可据此判定“logic 已接收”。
2. 非法请求返回明确失败，不接受模糊成功。
3. 对重复结算请求，允许当前版本先返回幂等成功或明确重复语义，但行为必须固定、可文档化、可测试。

兼容性影响：

- 不轻易改动 battle 已使用的 settle 请求主体结构。
- 如需为幂等或回包补字段，保持 battle 侧最小改造成本。

### 验收标准

1. **合法请求被接收**
   - 输入：battle 发送字段完整的 `S2S_BATTLE_SETTLE_REQ`
   - 期望：logic 完成最小校验并返回成功 `S2S_BATTLE_SETTLE_RSP`
   - 期望：日志或内部状态可确认本次 room/battle 已处理或已接收

2. **非法请求被拒绝**
   - 输入：缺少关键标识或请求结构明显不合法的 `S2S_BATTLE_SETTLE_REQ`
   - 期望：logic 返回明确失败
   - 期望：不进入后续发奖占位路径

3. **重复请求有固定处理语义**
   - 输入：同一 `room_id + battle_id` 的结算请求重复到达
   - 期望：logic 按既定幂等策略处理
   - 期望：battle 能从 rsp 或日志判断该次重复请求没有造成二次处理

### 风险

1. 本期不做持久化幂等，只能做到最小占位，服务重启后的重复保护能力有限。
2. 如果 success / duplicate / invalid 三类回包语义不清，battle 侧无法稳定收敛结束流程。
3. 后续发奖若直接叠加在当前 handler 上，容易把 P0 接收入口重新做乱，因此本次需要留清晰扩展边界。

---

## 本次统一不做范围

1. 不补 P0-1 房间生命周期与 tick 主循环详细产品拆分。
2. 不补 P0-2 怪物/波次生成推进详细产品拆分。
3. 不补 P0-3 结束条件定义细节，只承接其结果。
4. 不做正式 token 安全方案。
5. 不做完整断线重连。
6. 不做复杂塔/怪 combat 数值系统。
7. 不做完整结算发奖与持久化落库。

## 对 dev 的交接说明

1. 本次只按 P0-4 / P0-5 / P0-6 三个目标推进，不要顺手扩成 battle 全量重构。
2. 如实现中发现 README、`.agents/server.md` 与代码事实不一致，以代码事实为准，并在开发/联调阶段同步修正对应文档。
3. 任何阶段状态以协作文档为准，不默认口头“已经打通”。dev 完成后请更新 `[handoff-dev-to-test](E:\work\heli\server\.agents\coordination\handoff-dev-to-test.md)` 与 `[status-board](E:\work\heli\server\.agents\coordination\status-board.md)`。

## 风险/阻塞

1. battle 第二阶段依赖前置结束态/房间运行态已有最小可用实现；若代码事实不足，只能做必要衔接，不能无限扩任务。
2. README 仍保留旧 MVP 描述，dev/test 联调时要以当前代码和协作文档为准。
3. 当前 P0 边界明确不覆盖正式安全、正式幂等持久化、正式发奖；实现与测试结论中都要避免误报“已完整上线”。
