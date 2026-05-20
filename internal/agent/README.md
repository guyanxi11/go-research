# internal/agent

Phase 2 在此实现多 Agent 角色与 DAG 调度引擎：

- `planner.go` —— 把用户问题拆解为 DAG（节点 = 子问题，边 = 依赖）
- `researcher.go` —— ReAct 循环（Thought → Tool → Observation → Loop）
- `critic.go` —— 校验子报告，不过关则触发重跑
- `writer.go` —— 汇总为带引用的 Markdown 报告
- `dag/` —— 拓扑排序 + goroutine 并发 + 重试/超时/context 取消

Phase 1 暂未启用。
