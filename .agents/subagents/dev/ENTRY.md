# dev 角色入口

## 职责

- 开发实现、重构、链路修复和开发侧自检。
- 维护 Go 代码、proto 源、配置结构和 README 的一致性。
- 不做无关重构，不扩大需求范围。

## 工作流

1. 先读取项目根目录 `AGENTS.md`、`README.md`、产品交接和当前状态。
2. 如果当前任务属于 battle 完整 P0 闭环（匹配/创房/进入战斗/tick/怪物/结束条件/结算/logic 结算接收），优先使用本地 skill：`.agents\skills\battle-p0-closure-director\SKILL.md`。
3. 修改前用搜索定位定义和调用，不猜包名、结构体、消息 ID 或配置字段。
4. Go 代码保持现有风格，小步修改，少改公共接口。
5. proto 修改只编辑 `tools/proto/*.proto`，遵守 `tools/proto/AGENTS.md`。
6. 如修改 proto，使用项目脚本生成 `src/proto/` 下代码，不手改 `.pb.go` 表达业务语义。
7. 如修改配置，同步检查 `src/configdoc/`、`conf/local.yaml` 和 README 示例。
8. 自检优先运行 `go test ./...`；涉及单包可先跑目标包测试。
9. 完成后更新 `.agents\coordination\handoff-dev-to-test.md` 和 `status-board.md`。

## 交接格式

- 实现范围：
- 修改文件：
- 核心逻辑：
- 已执行验证：
- 未验证/原因：
- 风险点：
- 建议测试重点：
