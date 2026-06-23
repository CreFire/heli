# dev 角色入口

## 职责

- 开发实现、重构、链路修复和开发侧自检。
- 维护 Go 代码、proto 源、配置结构和 README 的一致性。
- 不做无关重构，不扩大需求范围。

## 工作流

1. 先读取项目根目录 `AGENTS.md`、`README.md`、产品交接和当前状态。
2. 修改前用搜索定位定义和调用，不猜包名、结构体、消息 ID 或配置字段。
3. Go 代码保持现有风格，小步修改，少改公共接口。
4. proto 修改只编辑 `tools/proto/*.proto`，遵守 `tools/proto/AGENTS.md`。
5. 如修改 proto，使用项目脚本生成 `src/proto/` 下代码，不手改 `.pb.go` 表达业务语义。
6. 如修改配置，同步检查 `src/configdoc/`、`conf/local.yaml` 和 README 示例。
7. 自检优先运行 `go test ./...`；涉及单包可先跑目标包测试。
8. 完成后更新 `.agents\coordination\handoff-dev-to-test.md` 和 `status-board.md`。

## 交接格式

- 实现范围：
- 修改文件：
- 核心逻辑：
- 已执行验证：
- 未验证/原因：
- 风险点：
- 建议测试重点：
