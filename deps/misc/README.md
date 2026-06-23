# misc
## 用途
承载杂项辅助函数，用于数值转换以及 `Number` / `Integer` 等共享接口。

## 适用场景
当需要跨多个包使用的通用数值格式化 / 解析能力时使用，例如 `IntToStr`、`StrToInt`、`StrToInt64` 和 `Number` 辅助能力。

## 避免使用场景
如果领域专用辅助函数已经覆盖同一转换路径，则不应使用。

## 关键入口
- `Number`、`Integer` 接口
- `IntToStr[T Integer](i T) string`
- `StrToInt`、`StrToInt64`

## 注意事项
该包刻意保持最小化；精确的舍入 / 转型行为请阅读代码。

## 业务使用
- `cache/server.go` 在 Redis hash 的在线人数和战斗负载快照处理中大量使用这些转换；服务启动代码也会读取 `BuildTime`、`ProgVer` 和 `ExcelVer`。
- 调用方认为这些辅助函数是服务代码中小型 int / string 转换的默认胶水。不要把它们误解为校验辅助函数；业务代码期望的是直接转换，而不是防御性解析。
- 在服务启动流程中看到 `misc` 时，通常表示版本 / 构建元数据暴露，而不是玩法逻辑。
