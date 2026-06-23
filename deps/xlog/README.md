# xlog 日志组件

`xlog` 是基于 [Zap](https://github.com/uber-go/zap) 封装的项目标准日志包，提供全局日志助手、结构化字段、运行时日志级别调整，以及按时间/大小轮转的文件写入能力。

## 使用场景

### 适合使用

- 需要项目统一格式的结构化日志。
- 需要包级别的全局日志函数，例如 `xlog.Infof`、`xlog.Errorw`。
- 需要在运行时调整日志级别。
- 需要日志文件按天/小时和大小自动轮转。

### 不建议使用

- 只需要临时本地打印。
- 只需要组件内部独立日志器，且不希望依赖全局状态。
- 面向用户的审计日志或关键业务流水日志；这些日志应使用业务专用通道。

## 关键入口

| 类型 | 入口 |
| --- | --- |
| 配置 | `Options` |
| 初始化默认日志器 | `InitDefaultLogger`、`InitDefaultLoggerWithOptions` |
| 创建独立日志器 | `MyLogger`、`NewMyLogger`、`NewMyLoggerWithOptions` |
| 包级日志函数 | `Debug*`、`Info*`、`Warn*`、`Error*` |
| 运行时控制 | `SetLogLevel`、`GetLogLevel`、`Sync`、`Close` |

> 使用包级别日志函数前，应在服务启动早期初始化默认日志器。文件轮转行为由 `rotate_writer.go` 中的 `Options` 控制。

## 日志配置

### Options 字段

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `FilePath` | `./logs/log` | 日志基础路径。实际写入文件会带上时间后缀，基础路径会尽量链接到当前日志文件。 |
| `Level` | `info` | 日志级别，支持 `debug`、`info`、`warn`、`error`、`dpanic`、`panic`、`fatal`。非法值回退到 `info`。 |
| `Rotation` | `daily` | 时间轮转粒度，支持 `daily`/`day` 和 `hourly`/`hour`。 |
| `MaxFileSizeMB` | `10` | 单个日志文件大小上限，超过后在当前时间槽内追加索引后缀。 |
| `RetentionDays` | `14` | 日志保留天数，超过保留期的历史文件会被清理。 |
| `Skip` | `2` | Zap caller skip 层级，用于修正调用行号。 |
| `Sync` | `false` | 是否同步写入。默认使用缓冲写入器，降低频繁写文件开销。 |
| `StdOut` | `true`* | 是否输出到标准输出。 |
| `FileOut` | `false`* | 是否输出到文件。 |

\* 当 `StdOut` 和 `FileOut` 都未开启时，`normalize()` 会默认开启 `StdOut`，避免日志完全无输出。`InitDefaultLogger(path, level)` 会显式开启 `FileOut`。

### 轮转规则

- `daily`：按天生成日志文件，文件名格式类似 `log.2026-06-23`。
- `hourly`：按小时生成日志文件，文件名格式类似 `log.2026-06-23-15`。
- 同一时间槽内超过 `MaxFileSizeMB` 时，追加索引后缀，例如 `log.2026-06-23.1`。
- 启动时会尝试接续最新的历史日志文件，避免重启后立刻丢失当前时间槽上下文。
- `RetentionDays` 到期的旧日志文件会在初始化或轮转时清理。

## 示例

### 初始化全局日志器

```go
xlog.InitDefaultLoggerWithOptions(xlog.Options{
    FilePath:      "./logs/logic",
    Level:         "info",
    Rotation:      "daily",
    MaxFileSizeMB: 20,
    RetentionDays: 14,
    Skip:          2,
    StdOut:        true,
    FileOut:       true,
})
```

### 记录结构化日志

```go
xlog.Infow("player login", "uid", uid, "ip", ip)
xlog.Warnf("retry request failed: %v", err)
xlog.Errorw("save player failed", "uid", uid, "error", err)
```

### 运行时调整日志级别

```go
xlog.SetLogLevel("debug")
```

## 业务约定

- `xlog` 是 `auth`、`gate`、`logic`、`public`、`query`、`robot` 等服务引导阶段使用的默认日志器。
- 业务代码默认假定服务启动前已经初始化包级别的 `xlog.*`。
- `configdoc`、`cache`、`persist` 中的日志属于基础设施日志。
- `service/logic/module/*`、`service/robot/controller/*` 中的日志主要用于请求状态调试。
- 高频 `Debug` 日志不是面向用户的审计日志，不应被当作业务流水使用。
- 调用方依赖共享的全局输出器和稳定字段名；不要随意修改已有字段含义。

## 注意事项

- 避免在热点循环中新增高频日志。
- 避免同一个错误在上层和下层重复记录，防止日志噪声过高。
- `Error*` 包级函数会主动调用 `Sync()`，用于尽量落盘错误日志。
- 服务退出前建议调用 `xlog.Close()`，确保缓冲区刷新并关闭文件句柄。
