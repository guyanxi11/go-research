# internal/sse

Phase 1 直接使用 `github.com/cloudwego/hertz/pkg/protocol/sse`，本目录暂未启用。

Phase 2 可能在此实现：

- 跨 Agent 的事件聚合（Plan 节点状态 / Researcher 进度 / Token 流多路复用）
- 客户端断线重连（Last-Event-ID 处理）
- SSE 事件 schema 的强类型化
