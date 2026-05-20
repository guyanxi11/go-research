# internal/tool

Phase 2 在此实现可插拔 Tool 系统。所有 Tool 实现统一 `Tool` 接口并通过注册中心暴露给 ReAct Agent：

- `search/tavily.go` —— Tavily Web 搜索
- `search/duckduckgo.go` —— DuckDuckGo（无 key 兜底）
- `fetch/readability.go` —— 网页正文抽取
- `fetch/pdf.go` —— PDF 解析
- `sandbox/docker.go` —— Phase 3+：Docker 沙箱代码执行（进阶项）

Phase 1 暂未启用。
