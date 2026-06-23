# xsync

## 用途
提供最小化同步辅助能力：并发 `Map`、低开销 mutex、分片原语和退避自旋器。

## 适用场景
当需要带 `GetOrNew` 的 goroutine 安全 map、受控删除能力，或希望防止循环长时间等待锁时使用。

## 避免使用场景
如果需要无锁数据结构，或要求超出当前自旋退避和 mutex 包装所提供能力的非阻塞保证，则不应使用。

## 关键入口
- `Map[K, V]`：围绕 `sync.RWMutex` 的包装，提供 `Set`、`Get`、`GetOrNew`、`GetOrSet`、`Delete`、`Range` 和 `RangeDelete`。
- `NewMap`：初始化受保护的 map。
- `spinning`：共享辅助函数，会按枚举步骤 yield / sleep 后再记录死锁。

## 注意事项
- `Range` / `RangeDelete` 会在锁内运行回调；辅助函数会把 panic 展开到日志，并在返回 `false` 时短路。
- 自旋器会先退避（`Gosched`、`Sleep`），再通过 `xlog` 提示潜在死锁。

## 业务使用
- `logic/actor/gamer_mgr.go`、`robot/controller/robotmgr.go` 和 `auth/controller/login_queue.go` 使用 `xsync.Map` 作为在线 session / player / login 状态的内存索引。
- 调用方假定它具备“共享可变注册表”语义，而不是无锁缓存。不要把 `Map` 误解为遍历成本低，或适合在内部执行重型回调；`Range` 仍然在锁内执行。
- 在这些业务路径中，关键不变量是通过 `sessId`、`gid` 或 `uid` 进行身份查找。应保持 API 使用直接，避免在其周围叠加额外抽象。
