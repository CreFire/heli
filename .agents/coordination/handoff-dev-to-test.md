# handoff-dev-to-test

## 本次开发内容

- 目标：补 battle 服 `BATTLE_JOIN_REQ` / `BATTLE_OP_REQ` handler 和 `battle_token` 校验骨架。
- 范围：
  - battle 客户端进房请求处理。
  - battle 局内操作请求处理。
  - 房间侧记录已进房 session。
  - 最小 token 校验骨架。
  - 操作成功后广播 battle delta。
- 不做范围：
  - battle token 签名/过期时间/防重放正式安全方案。
  - WebSocket/TCP 实机联调。
  - 断线重连恢复策略。
  - 真实战斗结束自动触发结算。

## 已实现

### 1. battle 进房与操作 handler

- 更新 `src/service/battle/handler.go`：
  - 注册 `BATTLE_JOIN_REQ`。
  - 注册 `BATTLE_OP_REQ`。
  - `reqBattleJoin(...)`：
    - 校验 `room_id` / `battle_token`。
    - 校验玩家是否属于房间。
    - 记录该玩家当前 battle session。
    - 返回 `S2CBattleJoinRSP`，附带当前快照 `snapshot`。
  - `reqBattleOp(...)`：
    - 校验房间存在。
    - 校验该玩家已完成 battle join 且 session 匹配。
    - 根据 op 类型调用 `sync.Room`：建塔 / 重随 / 合成 / 买矿工。
    - 成功后返回 `S2CBattleOpRSP`。
    - 同时广播 `S2CBattleDeltaNTF` 给房间内已进房玩家。

### 2. battle 房间数据补充

- 更新 `src/service/battle/room_manager.go`：
  - `battleRoom` 新增：
    - `allowedToken`
    - `joinedSess map[playerID]sessID`
  - `createRoom(...)` 增加 `battleToken` 入参。
  - 新增：
    - `hasPlayer(...)`
    - `bindPlayerSession(...)`
    - `matchPlayerSession(...)`

### 3. battle token 校验骨架

- 更新 `src/service/battle/battlesvr.go`：
  - 新增 `verifyBattleToken(...)`。
- 当前校验规则：
  - 房间存在。
  - `room_id` 一致。
  - 玩家属于该房间。
  - token 非空。
  - token 与建房时记录值一致。
  - token 前缀符合 `battle:<room_id>:`。

> 当前仍是占位安全方案，不是最终生产方案。

### 4. battle 广播骨架

- 更新 `src/service/battle/battlesvr.go`：
  - 新增 `sendProtoToSess(...)`
  - 新增 `broadcastRoomDelta(...)`
  - 操作成功后会把 `sync.Room.FlushDeltas()` 转成 `S2CBattleDeltaNTF` 下发给房间内已 join 的玩家。

## 已执行验证

```powershell
go test ./src/service/battle ./src/service/battle/sync
go test ./src/service/logic/module/match ./src/service/logic/module ./src/service/logic/...
```

结果：通过。

## 新增/更新测试

- 更新 `src/service/battle/battle_logic_bridge_test.go`：
  - 适配 createRoom 新签名。
  - 增加 `verifyBattleToken` 测试。

## 当前实现边界

- `battle_token` 现在只是“字符串一致性 + 前缀格式”校验。
- 还没有：
  - HMAC/签名
  - 过期时间
  - player_id 显式编码与校验
  - 一次性消费/防重放
- `BATTLE_OP_REQ` 当前只支持：
  - 建塔
  - 重随
  - 合成
  - 买矿工
- 进房成功后返回快照，但还未补低频心跳/重连补快照策略。

## 后续建议

1. 把 `battle_token` 升级为：`player_id + room_id + expire_at + sign`。
2. 补 battle 断线重连与重复 join 规则。
3. 把 `AdvanceToTick` 和周期性 delta 推送接到 battle 主循环。
4. 在房间结束时接 `settleRoom(...)` 做真实结算上报。
