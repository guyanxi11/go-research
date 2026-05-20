# GoResearch

> 一个用 Go 写的多 Agent 协作「深度研究助手」平台。
> 用户输入一个问题 → Planner 拆解子任务 → 多个 Researcher 并发检索 → Critic 校验 → Writer 输出带引用的报告。
>
> 当前进度：**Phase 1 — 骨架与单 Agent 流式问答**。

## 为什么是这个项目

- 主语言 Go 的并发模型（goroutine + channel）天然适合多 Agent 编排
- Deep Research 是当下 LLM 领域的一线热点（OpenAI / Google / 豆包都在投入）
- 简历亮点足够多：DAG 调度引擎、Worker Pool、令牌桶限流、全链路 SSE、可观测性

完整路线图见 [docs/PLAN.md](docs/PLAN.md)（后续补充）。

## 当前架构（最终目标）

```text
                                ┌──────────────────┐
   Web UI ── SSE ──▶ Hertz API ─┤  Orchestrator    │
                                │  / Planner Agent │
                                └────────┬─────────┘
                                         │ DAG
                                ┌────────▼──────────┐
                                │  Goroutine Pool   │
                                │  + Rate Limiter   │
                                └─┬───┬───┬─────────┘
                                  │   │   │
                                  ▼   ▼   ▼
                              Researcher × N (ReAct + Tools)
                                  │
                                  ▼
                               Critic ──▶ Writer ──▶ SSE 流式返回
```

Phase 1 只实现了：`UI → Hertz → /api/chat → Eino ChatModel → SSE`。其余角色在后续 Phase 接入。

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
├── cmd/server/         # 程序入口
├── internal/
│   ├── config/         # 配置加载（读 .env）
│   ├── llm/            # LLM 客户端（包裹 Eino ChatModel）
│   ├── server/         # Hertz 服务、路由、SSE 处理器
│   ├── agent/          # Phase 2：Planner / Researcher / Critic / Writer
│   ├── tool/           # Phase 2：搜索/抓取/PDF 等可插拔工具
│   └── sse/            # 预留：自定义 SSE 工具
├── web/                # 前端 Demo（Phase 3 替换为 Next.js）
├── scripts/init.sql    # Postgres 初始化（启用 pgvector）
├── configs/            # 预留：YAML/TOML 配置
├── docs/               # 文档
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

### 2. 起 Postgres + Redis（可选，Phase 1 不强依赖）

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

```bash
curl -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"hello"}'
```

## 改动你的 module 名

骨架默认用 `github.com/yourname/go-research`，发布到自己仓库前请改：

```bash
go mod edit -module github.com/<你的用户名>/go-research
# 然后批量替换 internal 包导入路径
```

## 路线图

- [x] **Phase 1** 骨架 + 单 Agent 流式问答
- [ ] **Phase 2** DAG 调度引擎 + 多 Agent（Planner/Researcher/Critic/Writer）+ Worker Pool 限流
- [ ] **Phase 3** 前端可视化（Plan 树、引用面板）+ pgvector 会话记忆
- [ ] **Phase 4** OpenTelemetry + Prometheus + Grafana + 压测
- [ ] **Phase 5** README/Demo 视频/K8s manifest/技术博客

## License

MIT
