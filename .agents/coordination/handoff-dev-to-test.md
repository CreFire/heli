# handoff-dev-to-test

- 实现范围：
  - 完成 battle P0 第二阶段的 P0-4 / P0-5 / P0-6。
  - P0-4：补强 battle join/op 最小校验链路，join 继续返回完整 snapshot；op 增加空 op_id、战斗已结束等失败语义，仍保持成功后先 rsp 再广播 delta。
  - P0-5：battle 房间结束后真实构建 settlement，并通过 battle->logic RPC 发送 `S2S_BATTLE_SETTLE_REQ`；同房间结算只发送一次；记录 logic 回包确认状态。
  - P0-6：logic 结算 handler 增加最小合法性校验、内存态幂等占位、固定回包语义，支持重复请求按幂等成功处理。
- 修改文件：
  - [E:\work\heli\server\src\service\battle\battlesvr.go](E:\work\heli\server\src\service\battle\battlesvr.go)
  - [E:\work\heli\server\src\service\battle\room_manager.go](E:\work\heli\server\src\service\battle\room_manager.go)
  - [E:\work\heli\server\src\service\battle\handler.go](E:\work\heli\server\src\service\battle\handler.go)
  - [E:\work\heli\server\src\service\battle\battle_stage2_test.go](E:\work\heli\server\src\service\battle\battle_stage2_test.go)
  - [E:\work\heli\server\src\service\logic\module\battle\handler.go](E:\work\heli\server\src\service\logic\module\battle\handler.go)
  - [E:\work\heli\server\src\service\logic\module\battle\handler_stage2_test.go](E:\work\heli\server\src\service\logic\module\battle\handler_stage2_test.go)
- 核心逻辑：
  - battle `finishBattleRoom` 不再只打结束日志，改为：防重 -> 构造 settlement -> 发送 logic -> 记录 ack/reject 结果。
  - `battleRoom` 增加 settlement ack/最近一次结算状态缓存，便于联调观察 battle 是否拿到 logic 回包。
  - `applyBattleOp` 增加 battle 结束态禁止操作、空 op_id 拒绝，避免结束后继续产生伪 delta。
  - logic `rpcBattleSettle` 校验 `room_id` / `battle_id` / tick 区间 / 玩家列表，并使用 `room_id:battle_id` 做内存幂等键；重复请求返回 accepted=true + `duplicate settle accepted`。
- 已执行验证：
  - `go test ./src/service/battle/...`
  - `go test ./src/service/logic/module/battle`
- 未验证/原因：
  - 未做 battle/logic 双进程实机联调，本次仅完成定向单测与包测试。
  - 未覆盖真实网络层 session/token 异常包收发观察，需 test 再做联调冒烟。
- 风险点：
  - logic 幂等当前仅内存态，服务重启后无法抵御重复 settle。
  - battle settle 失败仅记录状态和日志，未补重试队列，符合本期 P0 边界但联调时需关注。
  - join/op 仍是 P0 token/session 最小校验，不等于正式安全方案。
- 建议测试重点：
  - battle join 成功后 snapshot 是否完整。
  - 未 join / session 失配 / 已结束房间 op 是否明确失败。
  - battle 结束时是否只发一次 settle，logic 是否返回 accepted / duplicate / invalid 三类固定语义。

---

## 2026-06-24 task/shop 最小业务接入补充

### 本次追加范围

- 新增 logic task / shop 最小业务接入，基于已生成配置：
  - `docconf/daily_task_tbdailytask.json`
  - `docconf/gold_shop_tbgoldshop.json`
- 按当前代码结构补齐：
  - `task` 请求处理与最小模块实现
  - `shop` 请求处理与最小模块实现
  - `Gamer` / `iface` / `module_mgr` 接线
  - `micro.proto` 中缺失的 task/shop `MSG_ID`

### 新增/修改文件

- `tools/proto/micro.proto`
- `src/proto/pb/micro.pb.go`
- `src/service/logic/iface/interfaces.go`
- `src/service/logic/actor/gamer.go`
- `src/service/logic/module/module_mgr.go`
- `src/service/logic/module/shop/handler.go`
- `src/service/logic/module/task/handler.go`
- `src/service/logic/module/shopbiz/shop.go`
- `src/service/logic/module/shopbiz/shop_module.go`
- `src/service/logic/module/shopbiz/shop_module_impl.go`
- `src/service/logic/module/taskbiz/task.go`
- `src/service/logic/module/taskbiz/task_module.go`
- `src/service/logic/module/taskbiz/task_module_impl.go`

### 说明

- `shop/task` 采用类似 `match/matchbiz` 的拆分：
  - `shop` / `task` 包只保留 handler，避免和 `actor` 形成循环依赖
  - 真实业务实现下沉到 `shopbiz` / `taskbiz`
- `task` 当前实现为“最小配置驱动骨架”：
  - 首次访问时按 `TbDailyTask` 自动展开开启任务
  - `complete_value <= 0` 的任务直接置为可领奖
  - 奖励当前按 `reward_points -> 金币(configdoc.COIN_GOLD)` 发放
- `shop` 当前实现为“gold shop 最小骨架”：
  - 支持查询 tab 购买次数
  - 支持按 `TbGoldShop` 配置进行购买
  - 当前消费/发货都走金币道具 `configdoc.COIN_GOLD`

### 已执行验证

```powershell
go test ./src/service/logic/...
```

结果：通过。

### 当前风险/边界

1. `task` 当前只做了最小展开/领奖骨架，未接真实任务进度增长来源。
2. `shop` 当前直接以金币作为消费和奖励媒介，尚未抽象出更完整的商品/货币配置语义。
3. 本次为了让 task/shop 配置真正可走通，补充了 `micro.proto` 中缺失的 `MSG_ID`；后续如客户端已有固定号段约定，需要再统一复核。
4. 尚未补 task/shop 的专项单测；当前以 `go test ./src/service/logic/...` 编译通过为主。
