# mcp-searxng (Go Rewrite)

基于 **Go** 重写的 SearXNG MCP 服务端。

## 目标

- ✅ 只支持 **Streamable HTTP**（远程部署友好）
- ✅ 兼容浏览器/Studio 场景（含 CORS）
- ✅ 保留核心工具：
  - `searxng_web_search`
  - `web_url_read`

> 不再提供 stdio / legacy SSE 传输。

---

## 环境变量

### 必填

- `SEARXNG_URL`：你的 SearXNG 实例地址（如 `https://search.example.com`）

### 可选

- `MCP_HTTP_PORT`：HTTP 监听端口（默认 `8080`）
- `MCP_HTTP_PATH`：MCP 路径（默认 `/mcp`）
- `AUTH_USERNAME` / `AUTH_PASSWORD`：SearXNG Basic Auth
- `USER_AGENT`：请求头 User-Agent
- `URL_CACHE_TTL_SECONDS`：URL 读取缓存秒数（默认 `60`）
- `URL_FETCH_TIMEOUT_MS`：URL 抓取超时（默认 `10000`）
- `SEARCH_TIMEOUT_MS`：搜索超时（默认 `15000`）
- `READ_MAX_BYTES`：读取网页最大字节数（默认 `5242880`）
- `ALLOW_PRIVATE_ADDRESS`：`true` 时允许读取内网 URL（默认 `false`）

---

## 本地运行

```bash
go mod tidy
go run .
```

启动后：

- MCP Endpoint: `http://localhost:8080/mcp`
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

## 工具说明

### 1) `searxng_web_search`

输入：

- `query` (string, required)
- `pageno` (number, optional)
- `time_range` (`day|month|year`, optional)
- `language` (string, optional)
- `safesearch` (`0|1|2`, optional)

### 2) `web_url_read`

输入：

- `url` (string, required)
- `startChar` (number, optional)
- `maxLength` (number, optional)
- `section` (string, optional)
- `paragraphRange` (string, optional)
- `readHeadings` (boolean, optional)

---

## 安全策略

默认启用 URL 安全限制：

- 只允许 `http/https`
- 拒绝 localhost/内网地址
- DNS 解析后若命中私网 IP 也会拒绝

如需关闭（不建议生产）可设置：

```bash
ALLOW_PRIVATE_ADDRESS=true
```
