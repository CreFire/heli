# status-board

## 当前阶段

- 阶段：test 已完成 battle P0 第二阶段定向验收
- 负责人：test
- 更新时间：2026-06-24

## 当前任务

- 目标：仅完成 battle P0 第二阶段（P0-4 / P0-5 / P0-6）
- 当前已完成范围：
  - P0-1 房间生命周期与 tick 主循环
  - P0-2 写死波次/怪物生成与推进
  - P0-3 结束条件与结束状态固化
  - P0-4 battle 进房 / 操作 / 广播链路补强
  - P0-5 battle 结束触发结算上报
  - P0-6 logic 结算接收补强
- 不做范围：正式 token 安全方案、完整断线重连、复杂数值系统、配置表化、完整发奖落库

## 状态

| 角色 | 状态 | 说明 |
|------|------|------|
| product | 已完成 | 已输出第二阶段 P0-4 / P0-5 / P0-6 范围与验收标准 |
| dev | 已完成 | 已补 battle join/op 校验、结束结算上报、logic 最小校验与幂等占位 |
| ops | 已完成 | 已提供 battle/logic 最小运行说明，后续联调继续沿用 |
| test | 已完成 | 已完成定向自动化验收，battle 第二阶段通过 |

## 阻塞项

- README 与当前代码事实不一致，不能作为 battle 启动真相来源。
- 当前项目全量测试并非稳定基线，验收仍以 battle/logic 定向验证为主。
- 当前仓库缺少 battle 专用配置文件，battle 服务无法按统一 `-f` 方式直接启动。
- `conf/global.yaml` 当前配置 etcd 地址为 `127.0.0.1:1003`，联调前需先确认本机端口事实。
- battle settle 失败当前仅日志 + 内存态状态记录，尚无重试队列。
- logic 幂等当前仅进程内存态，服务重启后重复保护有限。

## 下一步

- 当前第二阶段定向自动化验收已通过，可继续安排 battle / logic 双进程联调。
- 联调优先关注：
  - battle join 成功回包中的 snapshot 字段是否与客户端接入预期一致
  - battle->logic settle RPC 在真实网络链路下的 accepted / duplicate / invalid 语义是否与单测保持一致
  - settle 失败场景下 battle 侧日志与内存态状态记录是否满足排障需要

## 架构决策记录

- 2026-06-23：合作塔防采用服务端权威状态同步。
- 2026-06-23：战斗开始后客户端直连 battle；logic 负责下发 `battle_addr`、`room_id`、`battle_token`。
- 2026-06-24：当前 battle 下阶段目标升级为“完整 P0 战斗闭环”，不再只停留在 join/op 骨架。
- 2026-06-24：ops 按当前代码事实确认，battle 第一阶段联调应优先收敛为 `logic <-> battle` 双服链路，不以旧 README 的单体启动说明为准。
- 2026-06-24：dev 已完成 P0-1~P0-3，当前 battle 房间已具备运行态、默认写死波次、最小胜负/超时/中止结束态与防重复 settle 标记。
- 2026-06-24：dev 已完成 P0-4~P0-6，battle 结束后会真实上报 settlement，logic 可做最小校验与内存幂等接收。
- 2026-06-24：test 已完成 battle P0 第二阶段定向自动化验收；join/op/settle 关键语义符合当前交接要求。

- 2026-06-24：dev 已补 task/shop 最小业务接入与缺失的 proto MSG_ID，`go test ./src/service/logic/...` 通过；当前仍属于配置驱动骨架，不代表完整任务系统/商城系统已完成。