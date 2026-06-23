---
name: logic-module-business-framework
description: "Use when: 在 goxianyu 中新增或重构 src/service/logic/module 下的玩家业务模块、handler、xxx_module.go、iface.I<Name>Module、IGamerContext、actor.Gamer 引用、module_mgr 注册、proto client/micro 协议接入。"
---

# goxianyu Logic 业务模块代码框架

用于让 AI 按统一格式编写 `src/service/logic/module/<module>` 业务模块。不要照搬旧服集中 dispatch；业务必须落到当前项目模块、协议和玩家上下文中。

## 必须遵守的三层文件结构

每个玩家业务模块优先按以下结构拆分：

1. `handler.go`
   - 只做 client 协议注册、请求解析、参数校验、从 `ctx` 取模块。
   - 不写业务规则、不直接操作存档。
   - 推荐 `type Handler struct{}`。
   - 请求函数返回 `(errorpb.ERROR, proto.Message)`。

2. `<module>_module.go`
   - 对外业务接口和方法承载层。
   - 定义 `<Name>Module`、`New<Name>Module(ctx iface.IGamerContext, model *gamedata.GamerModel)`。
   - 持有 `ctx iface.IGamerContext`、`model *gamedata.GamerModel`，以及内部数据对象或持久化模型。
   - 被 `iface.I<Name>Module` 暴露给其他模块和 handler 调用。

3. `<module>.go`
   - 内部存储、更新逻辑、私有领域函数。
   - 可定义 `<Name>Data`、数据更新函数、配置换算、奖励占位等。
   - 不直接注册协议。

## 标准接入清单

新增或重构模块时，按顺序检查：

1. 协议
   - `tools/proto/micro.proto` 添加 `MSG_ID_<MODULE>_<ACTION>_REQ/RSP`。
   - `tools/proto/client.proto` 添加明确 `REQ/RSP`。
   - 生成 `src/proto/pb` 后再写业务引用。

2. 模块接口
   - 在 `src/service/logic/iface/interfaces.go` 添加：
     - `type I<Name>Module interface { ... }`
   - 只暴露外部需要调用的方法，不暴露内部存储函数。
   - 如接口方法返回 proto，使用 `google.golang.org/protobuf/proto.Message`。

3. 玩家上下文
   - 在 `src/service/logic/iface/gamcontext.go` 的 `IGamerContext` 添加：
     - `<Name>() I<Name>Module`

4. `actor.Gamer` 引用
   - 在 `src/service/logic/actor/gamer.go`：
     - import `<name>module "game/src/service/logic/module/<name>"`
     - `Gamer` struct 添加 `<name>Module iface.I<Name>Module`
     - 添加访问器：`func (r *Gamer) <Name>() iface.I<Name>Module { return r.<name>Module }`
     - `bindModules()` 中初始化：`r.<name>Module = <name>module.New<Name>Module(r, r.Model)`
   - 如果只加 `IGamerContext` 不加 `Gamer` 方法，会导致 `*Gamer does not implement iface.IGamerContext`。

5. Handler 注册
   - `handler.go` 注册 `pb.MSG_ID_..._REQ`：
     - `r.CSRegister(pb.MSG_ID_XXX_REQ, ctxresolver.WrapC2S(m.reqXxx))`
   - 请求中从玩家上下文取模块：
     - `module := ctx.<Name>()`
     - `module == nil` 返回 `errorpb.ERROR_FAILED`
   - 参数错误返回 `errorpb.ERROR_REQUEST_PARAMS`。

6. ModuleMgr 注册
   - `src/service/logic/module/module_mgr.go` 添加 handler 字段、初始化和 `RegisterHandler` 调用。

7. 持久化
   - 有玩家存档才接 `src/persist/gamer_data.go`、`gamedata.RegisterMod`、默认数据。
   - 无真实存档的 P0 骨架不要强行加 `GamerData` 字段。

## 最小代码形态

`handler.go`：

```go
type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (m *Handler) RegisterHandler(rpc *rpcmgr.RpcMgr, r *router.Router) error {
    r.CSRegister(pb.MSG_ID_XXX_REQ, ctxresolver.WrapC2S(m.reqXxx))
    return nil
}

func (m *Handler) reqXxx(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
    req, ok := data.Message().(*pb.XxxREQ)
    if !ok || req == nil {
        return errorpb.ERROR_REQUEST_PARAMS, nil
    }
    module := ctx.Xxx()
    if module == nil {
        return errorpb.ERROR_FAILED, nil
    }
    return module.Xxx(req)
}
```

`xxx_module.go`：

```go
type XxxModule struct {
    ctx   iface.IGamerContext
    model *gamedata.GamerModel
    store *XxxData
}

func NewXxxModule(ctx iface.IGamerContext, model *gamedata.GamerModel) *XxxModule {
    return &XxxModule{ctx: ctx, model: model, store: NewXxxData()}
}

func (m *XxxModule) Xxx(req *pb.XxxREQ) (errorpb.ERROR, proto.Message) {
    if req == nil {
        return errorpb.ERROR_REQUEST_PARAMS, nil
    }
    if m == nil || m.store == nil {
        return errorpb.ERROR_FAILED, nil
    }
    return errorpb.ERROR_SUCCESS, &pb.XxxRSP{}
}
```

`xxx.go`：

```go
type XxxData struct{}

func NewXxxData() *XxxData { return &XxxData{} }
```

## 禁止事项

- 不要把普通业务继续塞进 `legacy.dispatch` 或 `MAIN_GM_REQ`。
- 不要在 `handler.go` 写业务和存档更新。
- 不要让 `Handler` 持有玩家模块实例；玩家模块必须从 `ctx.<Name>()` 获取。
- 不要混用 `github.com/gogo/protobuf/proto`，统一使用 `google.golang.org/protobuf/proto`。
- 未完成玩法不能静默发奖；P0 可返回明确失败或安全空结果。
- 调试接口如 `user.addbag` 不得注册为普通客户端接口。

## 验证要求

修改后至少执行：

- `gofmt` 格式化改动的 Go 文件。
- `go test ./src/service/logic/...`。
- 检查改动文件无诊断错误。
