# NeoBot

<p align="center">
  <img src="mm-chat/frontend/public/logo.png" width="96" alt="NeoBot 标志" />
</p>

<p align="center">
  <strong>面向多模型对话、Agent、Skill、MCP 工具、联网搜索、知识库 RAG、
  Memory、语音和产物的自托管 AI 工作台。</strong>
</p>

<p align="center">
  <a href="README.md">English</a>
  ·
  <a href="https://github.com/mumu-0922/NEObot/actions/workflows/ci.yml">CI</a>
  ·
  <a href="https://github.com/mumu-0922/NEObot/actions/workflows/docker.yml">Docker</a>
</p>

NeoBot 是以后端为权威、优先面向单服务器部署的 AI 工作台。Next.js UI
通过同源 `/mm-api` 访问私有 Go API；PostgreSQL 17 与 MinIO 保存持久状态，
Redis 承载非权威临时状态，Python Worker 负责文档解析与 RAG 任务。

产品源码全部位于 [`mm-chat/`](./mm-chat/)；仓库根目录仅保留 GitHub 与开发
工具所需的薄入口，不包含第二套应用。

## 功能特性

- 多 Provider 对话、按会话选择模型、分支、助理、文件附件与流式响应。
- 持久化 Agent Run，支持 Host Workspace 绑定、会话级 Skill / MCP 工具与
  显式 Permission Mode；Typed Timeline 与审批控件仍受 Rollout Gate 约束。
- Skill Library 与 Marketplace 安装、Remote MCP，以及用于受审本地 stdio
  Server 的可选 Hardened Runner。
- 安全网页读取与搜索、个人/团队知识库、带引用的 Grounded RAG，以及明确的
  失败与降级状态。
- 受治理的 Memory，支持作用域事实、Review / Undo、Activity 历史和加密
  Export / Import。
- 语音播放、图片生成、代码执行、Markdown、数学公式、Mermaid、引用与可编辑
  Artifact。
- 账号登录、密码找回/修改、Session 撤销、Provider Secret Vault、限流、
  Audit Trail 与最小权限 Runtime Role。

## 截图

![NeoBot 桌面工作台](mm-chat/frontend/public/desktop.png)

![NeoBot 移动工作台](mm-chat/frontend/public/mobile.png)

## 目录结构

```text
mm-chat/frontend/  Next.js 16 / React 19 前端
mm-chat/backend/   Go API、迁移、Worker 与运维命令
mm-chat/rag/       Python 文档解析与 RAG Worker
mm-chat/postgres/  PostgreSQL 17 BM25/pgvector 检索镜像
mm-chat/docs/      架构、契约、部署与恢复文档
mm-chat/scripts/   校验、发布、备份与恢复脚本
```

组件开发、Runtime Profile 与运维细节见
[`mm-chat/README.md`](./mm-chat/README.md)。

## 首次部署

基础要求为 Docker Engine 与 Compose v2。直接开发各组件时还需要
Node.js 22、pnpm 10.30.3、Go 1.25、Python 3.13 和 `uv`。

先准备受保护的本地配置与 non-root Agent Workspace：

```bash
cd mm-chat
cp .env.single-server.example .env.single-server
chmod 600 .env.single-server
mkdir -p data/agent-skills data/agent-workspace
chmod 700 data/agent-skills data/agent-workspace
./scripts/init-provider-keyring.sh
```

启动应用前，必须替换 `.env.single-server` 中的全部占位值，把
`MM_CHAT_RUNTIME_UID` / `MM_CHAT_RUNTIME_GID` 设为受保护文件的 Owner，执行
迁移，在全新数据库中创建四个最小权限 Runtime Login，并初始化首个 Owner
Identity。请严格遵循受审的
[`单服务器首次启动流程`](./mm-chat/docs/deployment/single-server-compose.md#local-development-first-boot)
与
[`全新安装 Role 配置`](./mm-chat/docs/deployment/postgres-single-server.md#fresh-install-role-provisioning)；
这些凭据步骤不能简化成把 Secret 塞入命令参数。

首次启动完成后，拉起浏览器可访问的 Stack：

```bash
docker compose --env-file .env.single-server \
  --profile app up -d --build
```

若未修改 `FRONTEND_PORT`，访问 <http://127.0.0.1:3000>。可选的 Memory、
RAG 与 MCP Runner Profile 见
[`mm-chat/docs/deployment/`](./mm-chat/docs/deployment/)。

## 验证

在仓库根目录运行隔离的完整 Gate：

```bash
bash mm-chat/scripts/verify-standalone.sh --full
```

也可独立运行各组件 Gate：

```bash
cd mm-chat/frontend
corepack pnpm install --frozen-lockfile
corepack pnpm format:check
corepack pnpm lint
corepack pnpm typecheck
corepack pnpm test
corepack pnpm build

cd ../backend
go vet ./...
go test ./...

cd ../rag
uv sync --frozen --all-groups
uv run ruff format --check .
uv run ruff check .
uv run mypy
uv run pytest
```

确定性的 Playwright Journey 覆盖 Auth、模型持久化、Agent Run、Memory、
Knowledge RAG、Skill 与 MCP，不需要 Provider Credential，也不会产生模型费用；
详见 [`mm-chat/frontend/e2e/README.md`](./mm-chat/frontend/e2e/README.md)。

## 安全与运维

- 将 [`mm-chat/.env.single-server.example`](./mm-chat/.env.single-server.example)
  视为配置 Schema，而非可直接部署的 Secret Material。
- 禁止提交 `mm-chat/.env.single-server`、`mm-chat/data/`、
  `mm-chat/secrets/` 与 `mm-chat/backup/`。
- 部署、密钥轮换、备份、恢复与回滚必须遵循
  [`mm-chat/docs/deployment/`](./mm-chat/docs/deployment/) 中的受审流程。
- 安全漏洞请通过
  [GitHub Security Advisories](https://github.com/mumu-0922/NEObot/security/advisories/new)
  私下报告。

## 许可证

[MIT](./LICENSE)
