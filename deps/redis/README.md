# redis

## 用途

封装面向游戏逻辑的 Redis/Rueidis 客户端、限流和分布式锁。

## 适用场景

服务中需要 Redis 连接、每日/模块限制检查，或锁原语（`RedLock*`）时。

## 避免使用场景

操作需要 helper 之外的高级 Redis 脚本，或只需要内存缓存时。

## 关键入口

- `NewRedisClient`, `NewRedisClientCluster`, `NewRedisClientWithRueidis` 用于获取 cmdable client。
- `RedLock`, `RedLockShort`, `RedLockShortWithTTL` 用于分布式锁。
- `CheckIncDailyLimit`, `CheckModuleKeyLimit`, `GetDailyLimit` 用于限流执行。
- `DailyLimit` 结构体和 limiter 配置 helper。

## 注意事项

使用后关闭/停止 client 以释放连接；通过调用方指定的 option 配置超时。

## 业务用法

- Gate 登录在账号 key 上使用 `RedLock` 串行化登录/重连/登出。加锁失败会被视为重复登录或竞争，而不是自动视为基础设施失败。
- `realm`、`worldboss` 和邮件等 public 模块使用短锁，确保同一时刻只有一个实例刷新或修改共享全局状态。
- 不要把这里的 Redis 锁当成长事务。调用代码假定临界区很短，并在竞争时立即回退为业务错误。

