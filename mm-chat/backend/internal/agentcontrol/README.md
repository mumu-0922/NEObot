# Agent control facade

`agentcontrol` 是 Agent Center 的 authenticated product facade。它把已有
Skill supply、Broker、Cron、Draft learning 与 PostgreSQL read models 组合成窄
HTTP/API 边界，而不把表 DML、Runner、Scheduler 或 learning-worker 身份交给
浏览器和 API process。

## 核心职责

- 按 session user 投影 Run、Step、Attempt、Event、Approval、Child 与 Artifact。
- 通过拥有 authority 的服务执行 Run cancellation、approval、Cron 和 Draft
  review，所有写入绑定 revision/generation/fingerprint。
- 返回 default-off Shadow policy/opt-in 状态，并以 boot generation、budget、
  Kill Switch 和 package/runtime fingerprint 拒绝 stale observation。
- 只在服务端解析 Artifact object key；客户端 DTO 不包含 bucket/key。
- 将 PostgreSQL/下游错误归一为稳定、脱敏的 Agent Center error codes。

## 依赖与边界

```text
authenticated HTTP
  -> Handler -> Service
       |         +-> agentbroker / agentcron / agentlearning
       +------------> PostgresRepository -> migration 090 views/functions
                                      +-> ObjectStore (owned Artifact only)
```

PostgreSQL 是 authority；Shadow adapter 是 injected seam。当前生产 wiring 不注入
adapter，Run enqueue 与 executable Shadow 均保持 `ISOLATION_UNAVAILABLE`。

## 使用

```go
service := agentcontrol.NewService(
    agentcontrol.WithRepository(agentcontrol.NewPostgresRepository(db)),
    agentcontrol.WithBroker(broker),
    agentcontrol.WithCron(cron),
    agentcontrol.WithLearning(learning),
    agentcontrol.WithArtifactStore(objects),
    agentcontrol.WithAdministratorUserID(adminID),
)
if err := service.Initialize(ctx); err != nil {
    return err // registers the restart/boot fence
}
handler := agentcontrol.NewHandler(service)
```

生产入口由 `backend/cmd/api/main.go` 建立服务，并由
`backend/internal/httpserver/server.go` 挂载 `/v1/agent-center/*`；不要绕过全局
session middleware 单独暴露 Handler。

## 验证

```bash
cd mm-chat/backend
go test -race ./internal/agentcontrol
go vet ./internal/agentcontrol
cd ..
bash scripts/verify-agent-product-shadow.sh
bash scripts/verify-agent-product-shadow-postgres17.sh
```

## 文件

- `handler.go`: route、strict JSON、download 与 sanitized error mapping。
- `service.go`: ownership、administrator、revision/fingerprint 与 held Runtime 规则。
- `repository_postgres.go`: migration `090` bounded views/functions adapter。
- `types.go`: DTO、service ports 与 stable errors。
- `*_test.go`: ownership、mutation binding、Kill Switch 与 object-key regression。

详见 [DESIGN.md](DESIGN.md) 与
[`docs/contracts/agent-runtime.md`](../../../docs/contracts/agent-runtime.md)。
