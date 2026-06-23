# async
## 用途
通过 `Async` 工作器对后台任务进行排队和执行，并跟踪待处理任务。

## 适用场景
当需要把载荷提交到 `Async` 队列中进行串行化、限速或重试时使用。

## 避免使用场景
如果简单同步调用即可满足需求，或已有调度器已经掌控执行流程，则不应使用。

## 关键入口
- `NewAsync(size int32, queueSize int32)`
- `(*Async).Post`、`(*Async).PostFixed`、`(*Async).PostFixedByString`
- `(*Async).Start`、`(*Async).Stop`

## 注意事项
`Stop` 会关闭工作器；关闭后不要复用该实例。
