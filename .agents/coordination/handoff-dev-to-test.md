# handoff-dev-to-test

## 本次开发内容

- 目标：开始编写 battle 战斗服的状态同步领域层。
- 范围：先落地 `src/service/battle/sync` 内存权威状态核心，不接网络、不改 proto、不生成 pb.go。
- 同步模式：服务端权威状态同步，客户端只提交操作意图。

## 已实现

- 新增 `src/service/battle/sync/room.go`：
  - `RoomConfig`：局内初始资源、建塔/重随/矿工成本、矿工产出、塔等级上限、玩家建造范围、默认塔池。
  - `Room`：维护玩家资源、塔、矿工、server_tick、增量事件。
  - `BuildTower`：校验玩家、建造范围、格子占用、金币；扣金币并生成 1 级随机塔。
  - `RerollTower`：只能重随自己的塔；扣魔力；保持等级不变并随机塔种类。
  - `MergeTower`：客户端提交 `main_tower_id` 与 `material_tower_id`；校验同玩家、同类型、同等级、未达等级上限；新塔保留主塔位置，材料塔位置清空。
  - `BuyMiner` / `AdvanceToTick`：花金币购买矿工，矿工按固定 tick 间隔产出固定魔力。
  - `Snapshot`：输出玩家、塔、矿工快照。
  - `FlushDeltas`：输出并清空资源变化、建塔、重随、合成、买矿工、矿工产出等增量事件。
- 新增 `src/service/battle/sync/room_test.go`：覆盖建塔、建造范围拒绝、重随、拒绝重随他人塔、合成位置规则、矿工产魔力。

## 已执行验证

```powershell
go test ./src/service/battle/sync
```

结果：通过。

尝试执行：

```powershell
go test ./...
```

结果：未完成全量通过，暴露项目既有编译/包缺口，与本次新增 `battle/sync` 包无关：

- `src/service/auth/bs.go` 引用缺失包 `game/src/backend`。
- `src/service/gate/gatesvr.go` 引用缺失包 `game/src/metrics`。
- `src/service/gate/module/login/handler.go` 引用缺失包 `game/src/common/bi`。
- `src/persist/public_gamer_data.go` 引用未定义 `pb.GamerMailDataNew`、`newGamerMailData`。

## 测试建议

- 先验收新增包：`go test ./src/service/battle/sync`。
- 若要跑 `go test ./...`，需要先修复上述既有缺失包/生成代码问题。

## 后续开发建议

1. 将 `sync.Room` 接入 battle 服务连接/房间管理。
2. 定义或补充 protobuf：建塔、重随、合成、买矿工、快照、增量事件、操作结果。
3. 接入 battle 直连鉴权：校验 logic/auth 下发的短期 battle token。
4. 后续扩展金币来源：杀怪、每波结算、自行召唤悬赏怪物。
