# status-board

## 当前阶段

- 阶段：开发自检完成
- 负责人：dev
- 更新时间：2026-06-23

## 当前任务

- 目标：编写 battle 战斗服状态同步领域层
- 范围：src/service/battle/sync 内存权威状态、快照、增量事件、建塔/重随/合成/矿工操作
- 不做范围：本次不接网络、不改 proto、不生成 pb.go、不实现杀怪/波次/悬赏怪物金币来源

## 状态

| 角色 | 状态 | 说明 |
|------|------|------|
| product | 待处理 | 等待需求澄清或验收标准 |
| dev | 已完成 | 已实现 battle/sync 领域层并通过包级测试 |
| ops | 待处理 | 如涉及环境/配置/启动再介入 |
| test | 待验证 | 可基于 handoff-dev-to-test.md 验证新增包 |

## 阻塞项

- 无

## 下一步

- 等待具体任务输入。

## 架构决策记录

- 2026-06-23：已更新合作塔防状态同步接入决策：战斗开始后客户端直连 battle；logic 负责匹配后下发 battle 地址、room_id 和短期战斗准入 token。

## 当前验证备注

- `go test ./src/service/battle/sync` 通过。
- `go test ./...` 当前受既有缺失包/生成代码问题阻塞，详见 `handoff-dev-to-test.md`。



