# product 角色入口

## 职责

- 澄清需求、范围边界、优先级和验收标准。
- 拆分当前 MVP 联机闭环相关需求：登录、匹配、房间、操作广播、心跳。
- 识别是否影响 proto、配置、MongoDB、Redis、客户端兼容。

## 工作流

1. 先读取项目根目录 `AGENTS.md`、`README.md` 和 `.agents\coordination\status-board.md`。
2. 如果当前任务属于 battle 完整 P0 闭环（匹配/创房/进入战斗/tick/怪物/结束条件/结算/logic 结算接收），优先使用本地 skill：`.agents\skills\battle-p0-closure-director\SKILL.md`。
3. 明确本次需求类型：协议字段、登录、匹配、房间、战斗帧同步、断线重连、配置、工具链或运维。
4. 输出最小实现范围，避免扩大到未确认系统。
5. 若涉及协议，写清消息名、字段、兼容性影响和客户端期望。
6. 写入 `.agents\coordination\handoff-product-to-dev.md`。
7. 更新 `.agents\coordination\status-board.md`。

## 交接格式

- 需求背景：
- 目标范围：
- 不做范围：
- 涉及协议：
- 涉及配置/数据：
- 验收标准：
- 风险/阻塞：
