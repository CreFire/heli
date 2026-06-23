# heli server AGENTS

## 全局约束

- 当前项目是 `heli server`，项目根目录：`E:\work\heli\server`。
- WSL 访问路径：`/mnt/e/work/heli/server`。
- 当前项目目录下的 `.agents/` 是本项目的本地协作目录。
- `.agents/Readme.md` 中记录了项目级硬约束：proto 调用统一使用 `google.golang.org/protobuf/proto`。
- `.agents/server.md` 是已有服务器项目文档；若发现它与当前代码不一致，以当前代码事实为准，并在任务中同步修正文档。
- 产品角色、开发角色、测试角色、运维角色、总代理通过本地文件协作，尽量保持渐进式、最小化。
- 开发不做过度防御设计；优先补齐当前联机闭环和明确的工程缺口。
- 文件引用统一使用 Markdown 绝对路径链接格式，例如：[AGENTS.md](`E:\work\heli\server\AGENTS.md`)。
- 项目使用 Go module，模块名为 `game`，当前 `go.mod` 声明 Go `1.26.4`。
- 项目当前是最小联机闭环：TCP/WebSocket 长连接、protobuf 协议、MongoDB、Redis、玩家登录、匹配、房间广播、心跳。
- 尽量使用中文。

## 项目结构速览

| 路径 | 说明 |
|------|------|
| `README.md` | 项目说明、配置、运行方式、MVP 指令与玩法闭环 |
| `conf/local.yaml` | 本地默认配置，包含 TCP、MongoDB、Redis、房间人数、tick 间隔 |
| `go.mod` / `go.sum` | Go 依赖声明 |
| `src/configdoc/` | 配置结构体定义，例如 server/net/auth/log |
| `src/proto/` | 由 proto 生成的 Go 代码，不优先手改 |
| `src/service/logic/` | 逻辑服务入口与业务逻辑承载目录 |
| `deps/netmgr/` | 网络管理、连接、会话、消息队列、TCP/WebSocket 传输 |
| `deps/msg/` | 消息头、消息序列化、protobuf 解析、WS 协议说明 |
| `deps/mongoclient/` | MongoDB 客户端与持久化辅助 |
| `deps/xlog/` | zap 日志与滚动写入 |
| `deps/fastid/` | ID 生成工具 |
| `deps/encrypt/` | AES 加密工具 |
| `tools/proto/` | proto 源文件与 proto 专用约定 |
| `tools/server_gen_proto.py` / `tools/gen_proto.ps1` | protobuf 生成脚本 |
| `tools/bin/` | 内置 Windows 版 protoc 与插件 |

## 子代理入口

| 子代理 | 职责 | 入口文件 |
|--------|------|----------|
| product | 需求澄清、玩法闭环拆分、验收标准、优先级说明、产品侧验收 | `E:\work\heli\server\.agents\subagents\product\ENTRY.md` |
| dev | 开发实现、重构、链路修复、开发侧自检、代码生成后的落地调整 | `E:\work\heli\server\.agents\subagents\dev\ENTRY.md` |
| test | 冒烟测试、边界测试、回归验证、协议兼容确认、验收确认 | `E:\work\heli\server\.agents\subagents\test\ENTRY.md` |
| ops | 本地启动依赖、配置检查、运行脚本、日志与故障定位 | `E:\work\heli\server\.agents\subagents\ops\ENTRY.md` |

## 最小协作文件

- 产品交接开发：`.agents\coordination\handoff-product-to-dev.md`
- 开发交接测试：`.agents\coordination\handoff-dev-to-test.md`
- 测试回传开发：`.agents\coordination\handoff-test-to-dev.md`
- 运维环境回传：`.agents\coordination\handoff-ops.md`
- 产品验收结论：`.agents\coordination\handoff-product-acceptance.md`
- 当前状态：`.agents\coordination\status-board.md`
- 总代理汇总：`.agents\coordination\final-summary.md`
- 需求与协议草稿：`.agents\protocol-and-gameplay-notes.md`
- 服务器项目文档：`.agents\server.md`
- 项目硬约束：`.agents\Readme.md`
- 本地技能：`.agents\skills\logic-module-business-framework\SKILL.md`

## 最小流程

1. `product` 梳理需求、范围、协议影响和验收标准后，更新 `handoff-product-to-dev.md` 和 `status-board.md`。
2. `dev` 先读取 README、相关 Go 文件、proto 文件和当前交接文档；完成开发和自检后，更新 `handoff-dev-to-test.md` 和 `status-board.md`。
3. 如改动运行环境、配置、数据库、Redis、监听地址或脚本，`ops` 补充环境说明，更新 `handoff-ops.md` 和 `status-board.md`。
4. `test` 基于交接内容做冒烟、边界和回归验证，更新 `handoff-test-to-dev.md` 和 `status-board.md`。
5. 如需产品验收，`product` 更新 `handoff-product-acceptance.md` 和 `status-board.md`。
6. 总代理最后读取所有协作文件，汇总到 `final-summary.md`。

## 角色工作流

### product

1. 明确本次目标属于哪一类：协议字段、登录、匹配、房间、战斗帧同步、断线重连、配置、工具链或运维。
2. 用最小范围描述需求，不提前扩展怪物波次、塔属性、结算等未确认系统。
3. 若涉及协议，列出消息名、字段、兼容性影响和客户端期望。
4. 给出可执行验收标准，例如：输入消息、预期响应、房间状态、Mongo/Redis 写入、广播内容。
5. 写入 `handoff-product-to-dev.md`，并在 `status-board.md` 标注阶段、阻塞项和优先级。

### dev

1. 修改前先定位相关文件，不猜包名、结构体、消息 ID 或配置字段。
2. Go 代码保持现有风格：小步修改、少改公共接口、避免无关重构。
3. 网络链路相关修改优先检查 `deps/netmgr/`、`deps/msg/` 和 `src/proto/` 的调用关系。
4. proto 修改只编辑 `tools/proto/*.proto`，再通过项目脚本生成 `src/proto/` 下的代码；生成文件不做手工业务修改。
5. `tools/proto/common.proto` 中只有需要持久化到 MongoDB 的结构体添加 `@gotags`，遵守 `tools/proto/AGENTS.md`。
6. 配置修改同步检查 `src/configdoc/`、`conf/local.yaml` 和 README 中的示例。
7. 自检优先运行：`go test ./...`；如涉及 proto 生成，再运行对应生成脚本并检查生成结果。
8. 完成后写清：改动文件、核心逻辑、已执行命令、未完成项、风险点。

### test

1. 先读取 `handoff-dev-to-test.md`、README、协议文件和相关实现。
2. 优先覆盖当前 MVP 闭环：连接、登录、匹配、房间广播、心跳。
3. 对协议变更检查：旧字段兼容、必填/默认值、消息 ID 或解析注册是否一致。
4. 对配置变更检查：默认 `conf/local.yaml` 是否可用，环境变量覆盖是否仍符合 README。
5. 能自动化则优先补 Go test；不能自动化时记录手工步骤、输入、期望和实际结果。
6. 验收后更新 `handoff-test-to-dev.md`：通过项、失败项、复现步骤、建议修复范围。

### ops

1. 检查本地依赖：MongoDB `127.0.0.1:27017`、Redis `127.0.0.1:6379`、服务监听 `:7001`。
2. 配置优先从 `conf/local.yaml` 和环境变量读取，避免把个人路径或机器专属配置写死进代码。
3. 运行或排障时记录命令、工作目录、环境变量和关键日志。
4. 生成 proto 时优先使用项目内置 `tools/bin`，注意当前脚本偏 Windows/PowerShell 路径。
5. 如果在 WSL 下运行 Windows `.exe` 或 PowerShell 脚本失败，记录失败原因并给出 Windows 终端可执行命令。

### 总代理

1. 分配任务前先读 `AGENTS.md`、`README.md`、当前协作文件和相关子目录 `AGENTS.md`。
2. 同步读取 `.agents/Readme.md`、`.agents/server.md` 和可用本地 skill，避免重复沉淀已有约定。
3. 只创建必要协作文件，避免扩展过多流程文档。
4. 阶段切换时要求上一个角色写清交接内容，不用口头状态替代文件状态。
5. 最终汇总包括：需求、实现、测试、运行方式、风险和后续建议。

## Proto 与协议约定

- `tools/proto/` 是 proto 源；`src/proto/` 是生成结果。
- `common.proto` 承载通用结构和需要持久化的结构，MongoDB tag 规则以 `tools/proto/AGENTS.md` 为准。
- `client.proto` 当前包含 Login、Match、PlayerOp、GameSnapshot、Heartbeat 等 MVP 客户端消息。
- `micro.proto` 当前包含基础 `MSG_ID` 枚举和服务间消息范围。
- Go 代码中 proto 调用统一使用 `google.golang.org/protobuf/proto`，不要引入 `github.com/gogo/protobuf/proto`。
- README 中的 MVP 指令表是客户端协议说明的一部分，协议变更后要同步更新。
- 不要手工修改 `.pb.go` 来表达业务语义；业务语义应回到 `.proto` 或业务代码。

## 验证建议

- 通用验证：`go test ./...`
- 指定包验证：`go test ./deps/netmgr/...`、`go test ./deps/msg/...`、`go test ./deps/mongoclient/...`
- 配置/启动验证：确认 MongoDB 和 Redis 可达后，再按 README 启动服务。
- Proto 生成验证：优先使用 `tools\gen_proto.ps1` 或 `python tools\server_gen_proto.py`，生成后检查 `src/proto/` 变更。

## 约定

- `product` 负责需求澄清、范围边界、优先级和验收标准。
- `dev` 负责实现和开发侧自检。
- `test` 负责验收和回传问题。
- `ops` 负责本地依赖、配置、启动和日志链路。
- 总代理负责流程推进和最终汇总。
- 任何阶段切换都尽量只改已有文件，不继续扩结构，避免上下文膨胀。
- 不提交、不推送、不改历史，除非用户明确要求。
