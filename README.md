# mcp-searxng (Pure Go)

纯 Go 版本的 SearXNG MCP 服务器（远程部署版）。

## 特性

- ✅ 仅支持 **Streamable HTTP**（不提供 stdio / Node 运行方式）
- ✅ 工具：
  - `searxng_web_search`
  - `web_url_read`
- ✅ URL 读取缓存 + SSRF 防护
- ✅ Studio / Web / Android LLM 客户端兼容优化：
  - 自动兼容 `Accept` 头（补齐 `application/json, text/event-stream`）
  - `JSONResponse` 优先（减少移动端 SSE 依赖）
  - CORS 预检与 MCP 头暴露

---

## 环境变量

### 必填

- `SEARXNG_URL`：你的 SearXNG 实例地址

### 可选

- `MCP_HTTP_PORT`：监听端口（默认 `8080`）
- `MCP_HTTP_PATH`：MCP 路径（默认 `/mcp`）
- `AUTH_USERNAME` / `AUTH_PASSWORD`：SearXNG Basic Auth
- `USER_AGENT`：自定义 User-Agent
- `URL_CACHE_TTL_SECONDS`：URL 缓存 TTL（默认 `60`）
- `URL_FETCH_TIMEOUT_MS`：URL 抓取超时（默认 `10000`）
- `SEARCH_TIMEOUT_MS`：搜索超时（默认 `15000`）
- `READ_MAX_BYTES`：读取网页最大字节（默认 `5242880`）
- `ALLOW_PRIVATE_ADDRESS`：允许读取内网地址（默认 `false`）
- `MCP_SESSION_TIMEOUT_SECONDS`：会话超时（默认 `1800`）
- `MCP_ANDROID_COMPAT`：移动端兼容（默认 `true`）

---

## 运行

```bash
go mod tidy
go run .
```

默认端点：

- MCP: `http://localhost:8080/mcp`
- Health: `http://localhost:8080/health`

---

## Docker

```bash
docker build -t mcp-searxng:go .
docker run --rm -p 8080:8080 \
  -e SEARXNG_URL=https://search.example.com \
  mcp-searxng:go
```

---

## Android LLM 客户端建议

如果客户端只走短连接 POST，不主动建立 SSE：

- 本服务已默认开启兼容模式（`MCP_ANDROID_COMPAT=true`）
- 服务会自动补齐常见 MCP 请求头，降低 400 概率
- 建议客户端仍尽量保留 `Mcp-Session-Id` 复用会话
