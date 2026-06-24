# battle P0 闭环说明

本文档用于沉淀当前 `battle` 目录下已经落地的最小战斗闭环实现，方便后续联调、接手和继续迭代。

## 1. 当前目标

当前 `battle` 服的职责是支撑合作塔防的 **P0 联机战斗闭环**：

1. logic 创建 battle 房间
2. 客户端直连 battle 并 join
3. 客户端提交局内操作
4. battle 房间自动推进 tick
5. battle 生成怪物、推进状态、判断结束
6. battle 结束后向 logic 上报结算

当前明确不做：

- 正式 token 安全方案
- 完整断线重连
- 配置表化
- 复杂 combat 数值
- 完整发奖/落库
- settle 失败重试队列

## 2. 目录内主要文件

> 2026-06-24 结构调整：
> battle 目录已开始做最小拆包，避免所有服务逻辑都堆在 `main` 包下。
> 当前建议理解为：
>
> - `src/service/battle/main.go`：仅保留服务启动入口
> - `src/service/battle/battleapp/`：battle 服务应用层
> - `src/service/battle/sync/`：局内权威状态同步领域层

### 核心入口

- [E:\work\heli\server\src\service\battle\main.go](E:\work\heli\server\src\service\battle\main.go)
- [E:\work\heli\server\src\service\battle\battleapp\battlesvr.go](E:\work\heli\server\src\service\battle\battleapp\battlesvr.go)

说明：

- `main.go` 只负责启动 battle 服务。
- `battleapp/battlesvr.go` 是 battle 服务应用层主体，负责房间 tick、delta 广播、结算发送。

### 协议处理

- [E:\work\heli\server\src\service\battle\battleapp\handler.go](E:\work\heli\server\src\service\battle\battleapp\handler.go)

说明：

- 注册 battle 的 rpc/cs handler
- 处理 logic -> battle 创房
- 处理 client -> battle join/op

### 房间管理

- [E:\work\heli\server\src\service\battle\battleapp\room_manager.go](E:\work\heli\server\src\service\battle\battleapp\room_manager.go)

说明：

- 管理 `battleRoom`
- 持有玩家列表、session 绑定、room loop、结算缓存
- 为 `sync.Room` 提供 battle 层包装

### 局内权威状态

- [E:\work\heli\server\src\service\battle\sync\room.go](E:\work\heli\server\src\service\battle\sync\room.go)
- [E:\work\heli\server\src\service\battle\sync\proto_adapter.go](E:\work\heli\server\src\service\battle\sync\proto_adapter.go)
- [E:\work\heli\server\src\service\battle\sync\settlement.go](E:\work\heli\server\src\service\battle\sync\settlement.go)

说明：

- `sync.Room` 是局内唯一权威状态
- 建塔、重随、合成、买矿工、怪物推进、结束条件、结算构造都在这里
- `proto_adapter.go` 负责快照、delta、怪物、结算与 proto 的转换

## 3. 当前链路

### 3.1 logic -> battle 创房

入口：

- `rpcCreateBattleRoom`

流程：

1. logic 发送 `S2S_BATTLE_CREATE_ROOM_REQ`
2. battle 生成 `battle_token`
3. battle 创建内存房间
4. battle 启动房间 tick loop
5. 返回 `room_id / battle_addr / battle_token`

### 3.2 client -> battle join

入口：

- `reqBattleJoin`

流程：

1. 客户端携带 `room_id / battle_token` 直连 battle
2. battle 校验：
   - 房间存在
   - room_id 一致
   - player 在房间内
   - token 非空且与房间匹配
3. 绑定 `player_id -> session_id`
4. 返回完整 snapshot

### 3.3 client -> battle op

入口：

- `reqBattleOp`
- `applyBattleOp`

流程：

1. 校验消息结构
2. 校验玩家已经 join 且 session 匹配
3. 校验房间未结束
4. 调用 `sync.Room` 执行玩法操作：
   - 建塔
   - 塔重随
   - 合成
   - 买矿工
5. 先返回 op rsp
6. 再广播 delta

## 4. tick 与状态同步

当前同步模型：**服务端权威状态同步**

说明：

- battle 不依赖客户端帧推进
- 每个房间独立 ticker，当前 tick 间隔固定为 `200ms`
- 每次 tick：
  1. 推进 `sync.Room`
  2. 收集并广播 delta
  3. 若房间结束，则触发结算

广播内容当前是：

- 资源变化
- 建塔/重随/合成
- 矿工产出
- 怪物状态变化
- 其他由 `sync.Room` 产生的增量

## 5. 结束与结算

### 5.1 房间结束

当前最小结束态已支持：

- `WIN`
- `LOSE`
- `ABORT`
- `TIMEOUT`（状态层已有最小支持）

### 5.2 结算上报

入口：

- `finishBattleRoom`
- `sendSettlement`

流程：

1. 房间进入 `CLOSED`
2. `settleOnce` 防止 battle loop 层重复进入
3. `MarkSettled()` 防止房间状态层重复构造结算
4. 构造 `Settlement`
5. 发送 `S2S_BATTLE_SETTLE_REQ` 给 logic
6. battle 缓存最近一次 settlement 以及 ack/reject 结果

当前限制：

- 失败仅记录日志和内存态错误
- 无重试队列
- logic 幂等仅进程内存态

## 6. 当前测试结论

截至 2026-06-24，battle P0 第二阶段定向自动化验收已通过，已确认：

- join 返回 snapshot
- 非法 op 失败语义
- 已结束房间禁止继续 op
- settle 只发送一次
- logic 对合法 / 非法 / 重复 settle 返回固定语义

相关协作文档：

- [E:\work\heli\server\.agents\coordination\handoff-dev-to-test.md](E:\work\heli\server\.agents\coordination\handoff-dev-to-test.md)
- [E:\work\heli\server\.agents\coordination\handoff-test-to-dev.md](E:\work\heli\server\.agents\coordination\handoff-test-to-dev.md)
- [E:\work\heli\server\.agents\coordination\status-board.md](E:\work\heli\server\.agents\coordination\status-board.md)

## 7. 下一步建议

如果继续推进，建议优先级如下：

1. 补 `battle` 专用配置文件，支持按统一 `-f` 启动
2. 做 battle / logic 双进程联调
3. 补 join 成功返回 snapshot 的显式单测/集成测试
4. 根据联调结果决定是否补 settle 重试或更明确的排障日志
5. 继续拆分 `battleapp` 内部职责，例如：
   - `handler`
   - `room_manager`
   - `settlement_sender`
   - `room_loop`

## 8. 当前拆包状态

本轮已完成：

- 将 battle 应用层实现从根目录 `main` 包迁移到 `battleapp` 子包：
  - `battleapp/battlesvr.go`
  - `battleapp/handler.go`
  - `battleapp/room_manager.go`
- `main.go` 改为仅依赖 `battleapp.App()` 启动服务
- battle 应用层测试同步迁移到 `battleapp/`

当前阻塞：

- `go test ./src/service/battle/battleapp` 受仓库现存文件 [E:\work\heli\server\deps\server\service.go](E:\work\heli\server\deps\server\service.go) 语法错误阻塞
- 已确认 `sync` 子包测试仍可通过

这意味着：

- 本次 battle 目录拆包已经落地
- 但 battleapp 的完整编译验证还需要先修复全局 `deps/server/service.go` 现存问题
