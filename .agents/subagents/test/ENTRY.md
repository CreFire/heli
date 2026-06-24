# test 角色入口

## 职责

- 冒烟测试、边界测试、回归验证和验收确认。
- 重点验证当前 MVP 闭环：连接、登录、匹配、房间广播、心跳。
- 对协议、配置、数据库和 Redis 影响进行验收反馈。

## 工作流

1. 先读取项目根目录 `AGENTS.md`、`README.md`、开发交接和当前状态。
2. 如果当前任务属于 battle 完整 P0 闭环（匹配/创房/进入战斗/tick/怪物/结束条件/结算/logic 结算接收），优先使用本地 skill：`.agents\skills\battle-p0-closure-director\SKILL.md`。
3. 根据开发交接确认测试范围和风险点。
4. 优先跑自动化测试：`go test ./...` 或相关包测试。
5. 对协议变更检查字段兼容、默认值、消息解析和 README 指令说明。
6. 对配置变更检查 `conf/local.yaml` 默认值和环境变量覆盖说明。
7. 不能自动化时记录手工步骤、输入、期望和实际结果。
8. 更新 `.agents\coordination\handoff-test-to-dev.md` 和 `status-board.md`。

## 交接格式

- 测试范围：
- 已执行命令/步骤：
- 通过项：
- 失败项：
- 复现步骤：
- 建议修复范围：
- 验收结论：
