# etcd

## 用途

围绕 `clientv3` 提供轻量封装，用于注册、发现和 watcher。

## 适用场景

需要注册/注销实例、监听服务 key，或驱动基于发现的负载均衡时。

## 避免使用场景

运行环境已经有不同的服务发现或注册层时。

## 关键入口

- `NewEtcdClient(dsn string, logger *xlog.MyLogger)`
- `NewClientWatcher`, `NewMultiWatcher`
- `NewRegistry(client *EtcdClient, regist bool)`
- `NewServiceRegistrar`, `RegistrarKey`, `ParseServerInfoFromKey`

## 行为

- 注册由 etcd 分布式锁保护，并拒绝重复实例 key。
- 服务 key 使用 lease 写入。更新时会先验证 key 仍属于注册器的 lease。
- `UpdateInstanceData` 会用调用方最新的实例快照替换运行时字段和 metadata。
- watcher 会先发送一份快照，再从后续 etcd revision 发送 put/delete 事件。
- watcher 错误会暴露给调用方，并通过新快照重试，因此 compaction 或重连不依赖本地补洞逻辑。
- watcher 投递使用背压而不是丢弃事件；关闭父 context 会在停机期间解除发送方阻塞。

## 注意事项

watcher 需要父 context，并且必须关闭以停止 goroutine。注册的启动和停止跟随调用方服务生命周期。

