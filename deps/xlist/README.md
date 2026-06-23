# xlist
## 用途
提供泛型双向链表，包含类型化节点以及移动 / 遍历辅助函数。

## 适用场景
当需要类似 `container/list` 的行为，同时希望使用泛型和稳定节点句柄时使用。

## 避免使用场景
如果 slice 更简单，或该结构必须并发安全，则不应使用。

## 关键入口
- `List[T]`、`Node[T]`
- `New`、`Init`
- `PushFront`、`PushBack`、`InsertBefore`、`InsertAfter`
- `MoveToFront`、`MoveToBack`、`Remove`
- `Range`、`Push`、`Pop`

## 注意事项
这是一个可变的侵入式链表。节点指针只要仍挂在同一个 list 上，就保持有意义。

## 业务使用
- `gate/gateuser/gate_user.go` 使用 `xlist.List[SendMessageInfo]` 作为等待客户端 ACK 消息的待发送队列。
- 不要把它当成可随意替换的通用集合。业务代码依赖重发 / 清理窗口中的有序保留语义，而不只是“这里有个列表”。
- 如果代理要修改队列行为，应先阅读 gate ACK 流程；简单替换为 slice 可能改变移除成本和节点稳定性假设。


