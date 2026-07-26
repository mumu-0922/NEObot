# NEObot / MM Chat

<p align="center">
  <img src="mm-chat/frontend/public/logo.png" width="96" alt="MM Chat 标志" />
</p>

<p align="center">
  <strong>由 Next.js 前端、Go API、PostgreSQL、私有对象存储和 Python RAG
  Worker 组成的自托管 AI 聊天系统。</strong>
</p>

<p align="center">
  <a href="README.md">English</a>
  ·
  <a href="https://github.com/mumu-0922/NEObot/actions/workflows/ci.yml">CI</a>
  ·
  <a href="https://github.com/mumu-0922/NEObot/actions/workflows/docker.yml">Docker</a>
</p>

产品源码全部位于 [`mm-chat/`](./mm-chat/)；仓库根目录仅保留 GitHub 与开发
工具所需的薄入口，不再包含第二套旧应用。

## 目录结构

```text
mm-chat/frontend/  Next.js 16 / React 19 前端
mm-chat/backend/   Go API、数据库迁移与运维命令
mm-chat/rag/       Python 文档解析与 RAG Worker
mm-chat/postgres/  PostgreSQL 17 检索镜像
mm-chat/docs/      架构、契约、部署与恢复文档
mm-chat/scripts/   校验、发布、备份与恢复脚本
```

完整架构、组件开发与运维说明见
[`mm-chat/README.md`](./mm-chat/README.md)。

## 快速启动

基础要求为 Docker Engine 与 Compose v2。直接开发各组件时还需要
Node.js 22、pnpm 10.30.3、Go 1.25 和 Python 3.13。

```bash
cd mm-chat
cp .env.single-server.example .env.single-server
chmod 600 .env.single-server
./scripts/init-provider-keyring.sh

docker compose --env-file .env.single-server \
  --profile ops run --rm migrate
docker compose --env-file .env.single-server \
  --profile app up -d --build
```

使用真实数据或 Provider 前，必须替换全部占位值。若未修改
`FRONTEND_PORT`，访问 <http://127.0.0.1:3000>。

## 验证

```bash
bash mm-chat/scripts/verify-standalone.sh --full

cd mm-chat/frontend
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

## 安全与运维

- 配置模板以
  [`mm-chat/.env.single-server.example`](./mm-chat/.env.single-server.example)
  为唯一权威来源。
- 禁止提交 `mm-chat/.env.single-server`、`mm-chat/data/`、
  `mm-chat/secrets/` 与 `mm-chat/backup/`。
- 部署、备份、恢复与回滚必须遵循
  [`mm-chat/docs/deployment/`](./mm-chat/docs/deployment/) 中的流程。
- 安全漏洞请通过
  [GitHub Security Advisories](https://github.com/mumu-0922/NEObot/security/advisories/new)
  私下报告。

## 许可证

[MIT](./LICENSE)
