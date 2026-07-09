# Reverse Proxy and TLS Runbook

This runbook covers the single-server production edge for `mm-chat`. Keep the
Go backend bound to `127.0.0.1:8080`; expose only the frontend origin through a
host-level reverse proxy with TLS.

## Boundary

```text
browser
  -> https://chat.example.com
  -> reverse proxy on 80/443
  -> Next.js frontend on localhost
  -> /mm-api/* stripped and proxied to 127.0.0.1:8080
  -> private Docker network: Postgres, Redis, MinIO
```

Rules:

- Publish only `80/tcp` and `443/tcp`; keep DB, Redis, MinIO, and the backend
  Docker network private.
- Prefer same-origin API access: configure the frontend with
  `NEXT_PUBLIC_API_MODE=server` and `NEXT_PUBLIC_API_BASE_URL=/mm-api`; the
  proxy forwards `/mm-api/*` to the Go backend root.
- The Go API does not emit CORS headers. Production should use same-origin
  `/mm-api`; direct browser cross-origin backend access requires a separate,
  explicit CORS allowlist design.
- Do not expose MinIO API or console publicly. Use SSH tunnel or VPN for admin
  access.
- Keep `/metrics` localhost-only or allowlisted. Do not expose it to the public
  internet.
- Preserve SSE streaming by disabling proxy buffering on API paths.
- Set upload limits at or above `MAX_UPLOAD_BYTES`.
- The Go rate limiter currently keys on the backend `RemoteAddr`; behind a
  local reverse proxy that may collapse clients to one proxy IP. If per-client
  public rate limiting is required, add it at the reverse proxy before traffic
  reaches the Go backend.

## Caddy Reference

Use Caddy when you want automatic certificate issuance and renewal. Replace
hostnames and frontend port placeholders before use.

```caddyfile
chat.example.com {
  encode gzip zstd

  header {
    X-Content-Type-Options nosniff
    Referrer-Policy no-referrer
    # Enable only after HTTPS is verified and rollback DNS is understood.
    # Strict-Transport-Security "max-age=31536000; includeSubDomains"
  }

  @metrics path /mm-api/metrics
  handle @metrics {
    respond "not found" 404
  }

  handle_path /mm-api/* {
    reverse_proxy 127.0.0.1:8080
  }

  reverse_proxy 127.0.0.1:3000
}
```

If Prometheus runs on the same host, scrape `http://127.0.0.1:8080/metrics`
directly instead of going through the public hostname.

## Nginx Reference

```nginx
server {
  listen 80;
  server_name chat.example.com;
  return 301 https://$host$request_uri;
}

server {
  listen 443 ssl http2;
  server_name chat.example.com;

  ssl_certificate /etc/letsencrypt/live/chat.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/chat.example.com/privkey.pem;

  add_header X-Content-Type-Options nosniff always;
  add_header Referrer-Policy no-referrer always;
  # Enable only after HTTPS is verified and rollback DNS is understood.
  # add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

  client_max_body_size 50m;

  location = /mm-api/metrics {
    allow 127.0.0.1;
    deny all;
    rewrite ^/mm-api/(.*)$ /$1 break;
    proxy_pass http://127.0.0.1:8080;
  }

  location /mm-api/ {
    rewrite ^/mm-api/(.*)$ /$1 break;
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 300s;
  }

  location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }
}
```

## Verification

```bash
curl -fsS https://chat.example.com/
curl -fsS https://chat.example.com/mm-api/health
curl -fsS https://chat.example.com/mm-api/ready
curl -fsS https://chat.example.com/mm-api/v1/version
curl -I https://chat.example.com/mm-api/metrics
```

Expected:

- `/mm-api/health`, `/ready`, and `/v1/version` work through TLS.
- Frontend server mode is enabled with:
  ```env
  NEXT_PUBLIC_API_MODE=server
  NEXT_PUBLIC_API_BASE_URL=/mm-api
  ```
- Streaming chat still receives incremental SSE frames; it must not wait for the
  full assistant response before rendering.
- File upload limit matches `MAX_UPLOAD_BYTES` and fails cleanly above it.
- Public `/mm-api/metrics` is blocked unless the request is from an allowlisted
  scraper path.

## Rollback

- Revert the proxy config and reload the proxy, not the data services.
- If frontend server-mode routing fails, set `NEXT_PUBLIC_API_MODE=local` and
  rebuild/restart the frontend to return to browser-local behavior while the
  backend remains on localhost.
- If TLS issuance fails, keep backend on localhost and use local/SSH testing
  until DNS and certificates are corrected.
- If SSE stalls after proxy changes, disable buffering on the API location and
  retest only the stream endpoint before widening traffic.
