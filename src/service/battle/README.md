# battle 目录说明

本文档是 `battle` 目录的开发接手说明，重点回答三个问题：

1. battle 目录里各文件/子包现在负责什么
2. 当前 battle P0 闭环是怎么跑起来的
3. 后续继续开发时应该优先改哪里

如果你想看更偏产品/流程视角的说明，请同时阅读：

- [E:\work\heli\server\src\service\battle\BATTLE_P0_OVERVIEW.md](E:\work\heli\server\src\service\battle\BATTLE_P0_OVERVIEW.md)

---

## 1. 当前目录结构

当前 `battle` 目录建议按三层理解：

### 1.1 启动层

- [E:\work\heli\server\src\service\battle\main.go](E:\work\heli\server\src\service\battle\main.go)

职责：

- 仅负责启动服务
- 不承载具体 battle 业务

### 1.2 应用层

- [E:\work\heli\server\src\service\battle\battleapp\battlesvr.go](E:\work\heli\server\src\service\battle\battleapp\battlesvr.go)
- [E:\work\heli\server\src\service\battle\battleapp\handler.go](E:\work\heli\server\src\service\battle\battleapp\handler.go)
- [E:\work\heli\server\src\service\battle\battleapp\room_manager.go](E:\work\heli\server\src\service\battle\battleapp\room_manager.go)

职责：

- 对接项目现有 `server.MS`
- 注册 battle 协议处理器
- 管理战斗房间
- 驱动 tick loop
- 广播状态增量
- 向 logic 发送结算

### 1.3 领域层

- [E:\work\heli\server\src\service\battle\sync\room.go](E:\work\heli\server\src\service\battle\sync\room.go)
- [E:\work\heli\server\src\service\battle\sync\proto_adapter.go](E:\work\heli\server\src\service\battle\sync\proto_adapter.go)
- [E:\work\heli\server\src\service\battle\sync\settlement.go](E:\work\heli\server\src\service\battle\sync\settlement.go)

职责：

- 维护局内权威状态
- 执行玩法规则
- 生成 snapshot / delta / settlement

---

## 2. battle 闭环主链路

### 2.1 创房

入口：

- `battleapp.rpcCreateBattleRoom`

说明：

- logic 发起 `S2S_BATTLE_CREATE_ROOM_REQ`
- battle 创建内存房间
- battle 启动自动 tick loop
- battle 返回 `battle_addr / room_id / battle_token`

### 2.2 进房

入口：

- `battleapp.reqBattleJoin`

说明：

- 客户端直连 battle
- 带上 `room_id + battle_token`
- 校验通过后绑定 session
- 返回完整 snapshot

### 2.3 局内操作

入口：

- `battleapp.reqBattleOp`
- `battleapp.applyBattleOp`

说明：

- op 先做 session 校验
- 再路由到 `sync.Room`
- 成功先回 rsp，再广播 delta

### 2.4 自动推进

入口：

- `battleapp.startRoomLoop`
- `battleapp.runRoomTick`

说明：

- battle 每个房间独立 ticker
- 当前 tick 间隔固定 `200ms`
- 自动推进怪物、矿工、资源和结束状态

### 2.5 结束与结算

入口：

- `battleapp.finishBattleRoom`
- `battleapp.sendSettlement`

说明：

- 房间关闭后停止 tick
- 构造 settlement
- 发送给 logic
- 缓存 ack/reject 结果

---

## 3. 代码阅读建议

如果你要快速接手 battle，建议按这个顺序看：

1. [E:\work\heli\server\src\service\battle\README.md](E:\work\heli\server\src\service\battle\README.md)
2. [E:\work\heli\server\src\service\battle\BATTLE_P0_OVERVIEW.md](E:\work\heli\server\src\service\battle\BATTLE_P0_OVERVIEW.md)
3. [E:\work\heli\server\src\service\battle\battleapp\handler.go](E:\work\heli\server\src\service\battle\battleapp\handler.go)
4. [E:\work\heli\server\src\service\battle\battleapp\battlesvr.go](E:\work\heli\server\src\service\battle\battleapp\battlesvr.go)
5. [E:\work\heli\server\src\service\battle\battleapp\room_manager.go](E:\work\heli\server\src\service\battle\battleapp\room_manager.go)
6. [E:\work\heli\server\src\service\battle\sync\room.go](E:\work\heli\server\src\service\battle\sync\room.go)

---

## 4. 当前设计边界

本目录当前是 **P0 联机闭环实现**，不是正式完整战斗框架。

已明确暂不处理：

- 正式 token 安全
- 完整断线重连
- 配置表化
- 复杂 combat 数值
- settle 持久化重试
- 完整结算发奖

所以如果后续继续开发，要尽量遵守这个原则：

- 优先补链路闭环
- 少做无关抽象
- 玩法规则尽量放进 `sync`
- 应用层只做协议、调度、广播、收口

---

## 5. 当前已知阻塞

当前 battle 目录已有最小拆包，但完整编译验证仍受全局问题影响：

- [E:\work\heli\server\deps\server\service.go](E:\work\heli\server\deps\server\service.go)

已知问题：

- 存在语法错误
- 会阻塞 `battleapp` 包的完整 `go test`

当前已知可确认：

- `sync` 子包测试可继续单独验证

---

## 6. 后续推荐拆分

如果继续整理 battle 目录，建议顺序如下：

1. 从 `battlesvr.go` 拆出 `settlement_sender.go`
2. 从 `room_manager.go` 拆出 `room_loop.go`
3. 为 `join success snapshot` 增加显式测试
4. 补 battle 专用配置文件
5. 再做 battle / logic 双服联调
