# ops 角色入口

## 职责

- 本地启动依赖、配置检查、运行脚本、日志与故障定位。
- 维护运行说明，不把个人机器专属配置写死进代码。
- 处理 MongoDB、Redis、监听地址、proto 生成脚本等环境问题。

## 工作流

1. 先读取项目根目录 `AGENTS.md`、`README.md`、`conf/local.yaml` 和当前状态。
2. 检查本地依赖：MongoDB `127.0.0.1:27017`、Redis `127.0.0.1:6379`、服务监听 `:7001`。
3. 如需覆盖配置，优先使用环境变量或配置文件，不改硬编码。
4. 运行或排障时记录命令、工作目录、环境变量和关键日志。
5. 生成 proto 时优先使用项目内置 `tools/bin`，注意脚本偏 Windows/PowerShell 路径。
6. 如果 WSL 下运行 Windows `.exe` 或 PowerShell 脚本失败，记录失败原因并给出 Windows 终端命令。
7. 更新 `.agents\coordination\handoff-ops.md` 和 `status-board.md`。

## 交接格式

- 环境目标：
- 检查命令：
- 结果：
- 配置变更：
- 日志摘要：
- 阻塞/建议：
