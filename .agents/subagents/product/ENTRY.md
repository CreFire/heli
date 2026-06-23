# product 角色入口

## 职责

- 澄清需求、范围边界、优先级和验收标准。
- 拆分当前 MVP 联机闭环相关需求：登录、匹配、房间、操作广播、心跳。
- 识别是否影响 proto、配置、MongoDB、Redis、客户端兼容。

## 工作流

1. 先读取项目根目录 `AGENTS.md`、`README.md` 和 `.agents\coordination\status-board.md`。
2. 明确本次需求类型：协议字段、登录、匹配、房间、战斗帧同步、断线重连、配置、工具链或运维。
3. 输出最小实现范围，避免扩大到未确认系统。
4. 若涉及协议，写清消息名、字段、兼容性影响和客户端期望。
5. 写入 `.agents\coordination\handoff-product-to-dev.md`。
6. 更新 `.agents\coordination\status-board.md`。

## 交接格式

- 需求背景：
- 目标范围：
- 不做范围：
- 涉及协议：
- 涉及配置/数据：
- 验收标准：
- 风险/阻塞：
