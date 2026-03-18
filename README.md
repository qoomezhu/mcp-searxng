# mcp-searxng (Netlify 适配版)

这个分支把原始的纯 Go MCP 服务器改成了 **Netlify Functions 适配版本**。

## 这个版本做了什么

- 新增 `netlify/functions/mcp/mcp.go` 作为 Netlify 部署入口
- MCP 运行在 **stateless + JSONResponse** 模式，适合 Netlify Functions
- 提供友好路由：`/mcp`、`/health`
- 同时保留直连入口：`/.netlify/functions/mcp`
- 增加 `public/index.html` 与重写规则，方便 Netlify 托管

## 部署前需要设置的环境变量

### 必填

- `SEARXNG_URL`：你的 SearXNG 实例地址

### 可选

- `AUTH_USERNAME` / `AUTH_PASSWORD`：SearXNG Basic Auth
- `USER_AGENT`：自定义 User-Agent
- `URL_CACHE_TTL_SECONDS`：URL 缓存 TTL，默认 `60`
- `URL_FETCH_TIMEOUT_MS`：URL 抓取超时，默认 `10000`
- `SEARCH_TIMEOUT_MS`：搜索超时，默认 `15000`
- `READ_MAX_BYTES`：读取网页最大字节，默认 `5242880`
- `ALLOW_PRIVATE_ADDRESS`：允许读取内网地址，默认 `false`
- `MCP_SESSION_TIMEOUT_SECONDS`：会话超时，默认 `60`
- `MCP_HTTP_PATH`：MCP 路径，默认 `/mcp`

## Netlify 部署步骤

1. 把这个分支连接到 Netlify
2. 在 Netlify 控制台里添加上面的环境变量
3. 发布

部署完成后可用地址：

- `https://你的域名/mcp`
- `https://你的域名/health`
- `https://你的域名/.netlify/functions/mcp`

## 本地开发

```bash
npx netlify dev
```

本地访问：

- `http://localhost:8888/mcp`
- `http://localhost:8888/health`

## 实现说明

这个 Netlify 版本和原始 Docker/常驻进程版本相比，做了一个关键调整：

- **不再依赖长连接 SSE 作为主要交互方式**
- 改成 **无状态 MCP + JSON 响应**，更符合 Netlify Functions 的运行模型

如果你想保留原始的 Docker 常驻进程方式，仓库根目录里的原始 Go 实现仍然可以作为参考。
