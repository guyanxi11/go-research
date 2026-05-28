# GoResearch

> 一个用 Go 写的多 Agent 协作「深度研究助手」平台。
> 用户输入一个问题 → Planner 拆解子任务 → 多个 Researcher 并发检索 → Writer 输出带引用的报告。
>
> 当前进度：**Phase 4 — 多轮 ReAct + Critic 自评回退**（在 Phase 3 持久化 / 缓存 / 历史 API 之上）。
>
> 端到端实测：单次 research 拉起 3 个并发 Researcher、Writer 流式输出 1769 tokens / 7376 字 / 18.1s 完成。

## 为什么是这个项目

- 主语言 Go 的并发模型（goroutine + channel）天然适合多 Agent 编排
- Deep Research 是当下 LLM 领域的一线热点（OpenAI / Google / 豆包都在投入）
- 简历亮点足够多：DAG 调度引擎、Worker Pool、令牌桶限流、全链路 SSE、可观测性

完整路线图见 [docs/PLAN.md](docs/PLAN.md)（后续补充）。

## 当前架构

```text
                                ┌──────────────────┐
   Web UI ── SSE ──▶ Hertz API ─┤  Orchestrator    │  ✅
                                │  + Planner Agent │  ✅
                                └────────┬─────────┘
                                         │ DAG (cycle-checked)
                                ┌────────▼──────────┐
                                │  Goroutine Pool   │  ✅
                                │  + Rate Limiter   │  ✅
                                └─┬───┬───┬─────────┘
                                  │   │   │
                                  ▼   ▼   ▼
                              Researcher × N            ✅
                              (search tool + LLM 综合)
                                  │
                                  ▼
                               Critic (Phase 4) ──▶ Writer ──▶ SSE 流式返回  ✅
```

## 已实现功能清单

- **`POST /api/chat`** — 单轮流式对话，前端会话内多轮历史，TTFT 计数
- **`POST /api/research`** — 全 pipeline SSE 端点，事件类型：`plan / node_started / node_finished / node_failed / writer_token / done / error`
- **DAG 调度引擎**（`internal/agent/dag`）
  - 三色 DFS 环检测、缺依赖检测、自环检测
  - share-by-communicating 模式（单 coordinator + N worker，无 mutex）
  - 节点级 retry（指数退避，ctx-aware）+ timeout
  - Fail-fast 取消同级、panic 恢复、`sync.WaitGroup` 优雅退出
- **限流器**（`internal/ratelimit`）— 令牌桶 QPS + 信号量并发上限，Acquire/Release/TryAcquire
- **Tool 系统**（`internal/tool`）— 可插拔接口 + Registry，已接入 Mock 与 Tavily 两套搜索
- **多 Agent 协作**（`internal/agent`）
  - Planner — 严格 JSON 输出 + 容错解析 + 失败重试一次
  - Researcher — **多轮 ReAct 搜索**（按需追加 follow-up query，URL 去重保持插入顺序）+ LLM 综合，输出 findings + 行内引用
  - Critic — 对每个子任务的 findings 评分 1-10，低于阈值给出反馈让 Researcher 重做
  - Writer — 全局重编号引用，Token 流式输出
  - Orchestrator — 串起四阶段，统一 Event 流给 HTTP 层（`search_round` / `critic_review` 也走 SSE）
- **持久化与缓存**
  - Postgres：`research_sessions` + `research_tasks` 两张表，记录 plan / 子任务状态 / report / 错误，`sort_order` 仅在 INSERT 时写入避免被后续 UPSERT 抹平
  - Redis 搜索缓存：缓存 key 纳入 provider + depth + max_items + schema 版本，换 provider / 改 depth 不会拿到错的旧结果
- **安全 & 健壮性**
  - 可选 `X-API-Key` 鉴权（设置 `API_KEY` 后启用，未设置保持本地开发友好）
  - `/api/research` 总超时（`RESEARCH_TIMEOUT_SECONDS`，默认 180s）防止 LLM 挂死泄漏 goroutine
  - SSE 客户端断开时取消上游 pipeline，事件 channel 主动 drain
- **前端可视化**（`web/index.html`）
  - Tab 切换 Chat / Research 两种模式
  - Plan 树：每个子任务一张卡，状态点（pending/running/done/failed），实时耗时徽章
  - Findings：可折叠面板，引用号渲染
  - Report：rAF 节流 Markdown 实时渲染，避免 1700 token 打爆 DOM
- **测试覆盖**：21 个单元测试（DAG 7 + Scheduler 9 + Limiter 5），全部通过

## 技术栈

| 模块       | 选型                                             |
| ---------- | ------------------------------------------------ |
| Web 框架   | [Hertz](https://github.com/cloudwego/hertz)      |
| LLM 框架   | [Eino](https://github.com/cloudwego/eino)        |
| LLM 提供商 | DeepSeek / 豆包 / Kimi / OpenAI（任一 OpenAI 兼容） |
| 数据库     | PostgreSQL + pgvector                            |
| 缓存       | Redis                                            |
| 流式       | Server-Sent Events (SSE)                         |
| 部署       | Docker Compose                                   |

## 目录结构

```text
go-research/
├── cmd/server/                # 程序入口（依赖装配）
├── internal/
│   ├── config/                # 配置加载（读 .env）
│   ├── llm/                   # LLM 客户端（包裹 Eino ChatModel）
│   ├── ratelimit/             # 令牌桶 + 信号量
│   ├── server/                # Hertz 服务、路由、/api/chat、/api/research
│   ├── agent/
│   │   ├── dag/               # DAG 调度引擎（Graph + Scheduler + 测试）
│   │   ├── planner/           # 拆题 → 子任务 DAG
│   │   ├── researcher/        # 单子任务搜索 + 综合
│   │   ├── writer/            # 流式 Markdown 报告 + 引用合并
│   │   └── orchestrator/      # Pipeline 编排 + 统一事件流
│   └── tool/
│       ├── tool.go            # Tool 接口 + Registry
│       └── search/            # Mock + Tavily 两个适配器
├── web/index.html             # 前端 Demo（Chat + Research Tab）
├── scripts/init.sql           # Postgres 初始化（启用 pgvector，Phase 3 用）
├── docker-compose.yml
├── Makefile
└── .env.example
```

## 快速开始

### 1. 准备依赖

```bash
git clone <your-repo>
cd go-research
cp .env.example .env
# 在 .env 中填上 LLM_API_KEY；其余默认即可
```

LLM API key 推荐去 [DeepSeek 开放平台](https://platform.deepseek.com/) 申请，每月免费额度够开发用。

### 1.1 接 Tavily 真搜索（Research 推荐）

不配置时 Research 使用 **Mock 搜索**（假链接）。要真实联网检索：

1. 打开 [Tavily](https://app.tavily.com/) 注册并创建 API Key（`tvly-...`）
2. 写入 `.env`：

```env
TAVILY_API_KEY=tvly-你的key
TAVILY_SEARCH_DEPTH=basic   # 或 advanced（更准、更耗额度）
```

3. 验证 key 是否可用：

```bash
make tavily-smoke
```

4. 重启服务，启动日志应出现 `search tool: tavily`，浏览器 Chat 标签旁显示 **· Tavily**

### 2. 起 Postgres + Redis（Phase 3 必须；Docker 一键起，无需本机安装 Postgres）

```bash
make up
```

### 3. 起服务

```bash
make tidy   # 首次拉依赖
make run
```

打开浏览器访问 <http://localhost:8080>，输入问题点「发送」就能看到 SSE 打字机效果。

### 4. 验证 SSE 端点（命令行）

单轮 chat：

```bash
curl -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"hello"}'
```

完整研究 pipeline：

```bash
curl -N -X POST http://localhost:8080/api/research \
  -H "Content-Type: application/json" \
  -d '{"question":"对比 Hertz 与 Gin 在高并发场景下的差异"}'
```

可观察到的事件序列：
`plan` → 多个 `node_started` / `node_finished` 交替（并发 Researcher）→ 多个 `writer_token` → `done`。

## 改动你的 module 名

骨架默认用 `github.com/yourname/go-research`，发布到自己仓库前请改：

```bash
go mod edit -module github.com/<你的用户名>/go-research
# 然后批量替换 internal 包导入路径
```

## 路线图

- [x] **Phase 1** 骨架 + 单 Agent 流式问答（Hertz + Eino + SSE）
- [x] **Phase 2.A** DAG 调度引擎 + 令牌桶限流 + 21 个单测
- [x] **Phase 2.B** Planner / Researcher / Writer / Orchestrator + `/api/research` SSE
- [x] **Phase 2.C** 前端可视化：Tab 切换、计划树、流式 Markdown 报告
- [x] **Phase 3** Postgres 落库（plan / findings / report）+ Redis 搜索缓存 + `GET /api/research` 历史 API + History 页
- [x] **Phase 4** Researcher 多轮 ReAct + Critic 评分回退 + `search_round` / `critic_review` SSE 事件 + 可选 X-API-Key 鉴权 + research 总超时
- [ ] **Phase 3.5** pgvector 向量记忆（RAG / 长期记忆）
- [ ] **Phase 5** OpenTelemetry + Prometheus + Grafana + 压测
- [ ] **Phase 6** Docker 镜像 + 一键部署到 Railway/Fly.io + Demo 视频 + 技术博客

## License

MIT
