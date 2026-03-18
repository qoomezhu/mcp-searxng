package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "qoomezhu/mcp-searxng-netlify"
	serverVersion = "2.0.0-netlify"
)

type Config struct {
	SearxngURL          string
	AuthUsername        string
	AuthPassword        string
	UserAgent           string
	MCPPath             string
	CacheTTL            time.Duration
	FetchTimeout        time.Duration
	SearchTimeout       time.Duration
	MaxBodyBytes        int64
	AllowPrivateAddress bool
	SessionTimeout      time.Duration
}

type Service struct {
	cfg      Config
	searxURL *url.URL
	client   *http.Client
	cache    *URLCache
	handler  http.Handler
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}

	svc, err := newService(cfg)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	lambda.Start(svc.handleLambdaEvent)
}

func newService(cfg Config) (*Service, error) {
	base, err := url.Parse(cfg.SearxngURL)
	if err != nil {
		return nil, fmt.Errorf("SEARXNG_URL 格式无效: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("SEARXNG_URL 只支持 http/https")
	}

	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}
	svc := &Service{
		cfg:      cfg,
		searxURL: base,
		client:   client,
		cache:    NewURLCache(cfg.CacheTTL),
	}
	svc.handler = buildHTTPHandler(svc)
	return svc, nil
}

func buildHTTPHandler(svc *Service) http.Handler {
	mcpServer := createMCPServer(svc)
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{
			Stateless:      true,
			JSONResponse:   true,
			SessionTimeout: svc.cfg.SessionTimeout,
		},
	)

	mux := http.NewServeMux()
	mux.Handle(svc.cfg.MCPPath, withMobileClientCompatibility(mcpHandler))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":             "ok",
			"server":             serverName,
			"version":            serverVersion,
			"transport":          "streamable-http",
			"responseMode":       "json",
			"stateless":          true,
			"endpoint":           svc.cfg.MCPPath,
			"functionEndpoint":   "/.netlify/functions/mcp",
			"allowPrivateAddress": svc.cfg.AllowPrivateAddress,
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":           serverName,
			"version":        serverVersion,
			"protocol":       "streamable-http",
			"responseMode":   "json",
			"stateless":      true,
			"mcpEndpoint":    svc.cfg.MCPPath,
			"healthEndpoint": "/health",
			"functionEndpoint": "/.netlify/functions/mcp",
		})
	})

	app := http.Handler(mux)
	app = withCORS(app)
	app = loggingMiddleware(app)
	return app
}

func createMCPServer(svc *Service) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, &mcp.ServerOptions{Logger: slog.Default()})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "searxng_web_search",
		Description: "Use SearXNG for web search with optional page/time/language/safesearch filters.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "搜索关键词"},
				"pageno": map[string]any{"type": "integer", "description": "页码，从1开始"},
				"time_range": map[string]any{"type": "string", "description": "时间范围: day/month/year", "enum": []string{"day", "month", "year"}},
				"language": map[string]any{"type": "string", "description": "语言代码，默认 all"},
				"safesearch": map[string]any{"type": "integer", "description": "安全搜索级别 0/1/2", "enum": []int{0, 1, 2}},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}, svc.handleSearchTool)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "web_url_read",
		Description: "Fetch URL content and convert HTML to markdown with heading/section/paragraph pagination options.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "要读取的 URL"},
				"startChar": map[string]any{"type": "integer", "description": "起始字符位置"},
				"maxLength": map[string]any{"type": "integer", "description": "最多返回字符数"},
				"section": map[string]any{"type": "string", "description": "按标题提取分节"},
				"paragraphRange": map[string]any{"type": "string", "description": "段落范围，如 1-5 / 3 / 10-"},
				"readHeadings": map[string]any{"type": "boolean", "description": "仅返回标题列表"},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
	}, svc.handleReadTool)

	return srv
}

func loadConfig() (Config, error) {
	cfg := Config{
		SearxngURL:          strings.TrimSpace(os.Getenv("SEARXNG_URL")),
		AuthUsername:        os.Getenv("AUTH_USERNAME"),
		AuthPassword:        os.Getenv("AUTH_PASSWORD"),
		UserAgent:           strings.TrimSpace(os.Getenv("USER_AGENT")),
		MCPPath:             getEnv("MCP_HTTP_PATH", "/mcp"),
		CacheTTL:            time.Duration(getEnvInt("URL_CACHE_TTL_SECONDS", 60)) * time.Second,
		FetchTimeout:        time.Duration(getEnvInt("URL_FETCH_TIMEOUT_MS", 10000)) * time.Millisecond,
		SearchTimeout:       time.Duration(getEnvInt("SEARCH_TIMEOUT_MS", 15000)) * time.Millisecond,
		MaxBodyBytes:        int64(getEnvInt("READ_MAX_BYTES", 5*1024*1024)),
		AllowPrivateAddress: getEnvBool("ALLOW_PRIVATE_ADDRESS", false),
		SessionTimeout:      time.Duration(getEnvInt("MCP_SESSION_TIMEOUT_SECONDS", 60)) * time.Second,
	}

	if cfg.SearxngURL == "" {
		return Config{}, fmt.Errorf("SEARXNG_URL 不能为空")
	}
	if (cfg.AuthUsername == "") != (cfg.AuthPassword == "") {
		return Config{}, fmt.Errorf("AUTH_USERNAME 与 AUTH_PASSWORD 必须同时设置")
	}
	if cfg.MaxBodyBytes <= 0 {
		return Config{}, fmt.Errorf("READ_MAX_BYTES 必须 > 0")
	}
	if cfg.SessionTimeout <= 0 {
		return Config{}, fmt.Errorf("MCP_SESSION_TIMEOUT_SECONDS 必须 > 0")
	}
	if !strings.HasPrefix(cfg.MCPPath, "/") {
		cfg.MCPPath = "/" + cfg.MCPPath
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func withCORS(next http.Handler) http.Handler {
	allowHeaders := strings.Join([]string{"Content-Type", "Accept", "Authorization", "Mcp-Session-Id", "Mcp-Protocol-Version", "Last-Event-ID"}, ",")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id,Mcp-Protocol-Version")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withMobileClientCompatibility(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodGet || r.Method == http.MethodDelete {
			normalizeAcceptHeader(r.Header)
		}
		if r.Method == http.MethodPost {
			if strings.TrimSpace(r.Header.Get("Content-Type")) == "" {
				r.Header.Set("Content-Type", "application/json")
			}
			if strings.TrimSpace(r.Header.Get("Mcp-Protocol-Version")) == "" {
				r.Header.Set("Mcp-Protocol-Version", "2025-03-26")
			}
		}
		next.ServeHTTP(w, r)
	})
}

func normalizeAcceptHeader(h http.Header) {
	accept := strings.TrimSpace(h.Get("Accept"))
	if accept == "" {
		h.Set("Accept", "application/json, text/event-stream")
		return
	}
	for _, want := range []string{"application/json", "text/event-stream"} {
		if !containsMime(accept, want) {
			accept += ", " + want
		}
	}
	h.Set("Accept", accept)
}

func containsMime(accept, want string) bool {
	for _, item := range strings.Split(accept, ",") {
		token := strings.TrimSpace(strings.Split(item, ";")[0])
		if strings.EqualFold(token, want) || token == "*/*" {
			return true
		}
	}
	return false
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func (s *Service) handleLambdaEvent(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	httpReq, err := requestFromEvent(ctx, req, s.cfg.MCPPath)
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusBadRequest, Headers: map[string]string{"Content-Type": "text/plain; charset=utf-8"}, Body: err.Error()}, nil
	}

	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, httpReq)
	resp := recorder.Result()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return events.APIGatewayProxyResponse{}, err
	}

	out := events.APIGatewayProxyResponse{StatusCode: resp.StatusCode, Headers: map[string]string{}, Body: string(body), IsBase64Encoded: false}
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			out.Headers[k] = strings.Join(vals, ",")
		}
	}
	if _, ok := out.Headers["Content-Type"]; !ok {
		out.Headers["Content-Type"] = "application/json; charset=utf-8"
	}
	return out, nil
}

func requestFromEvent(ctx context.Context, req events.APIGatewayProxyRequest, mcpPath string) (*http.Request, error) {
	method := strings.TrimSpace(req.HTTPMethod)
	if method == "" {
		method = http.MethodGet
	}
	path := req.Path
	if path == "" {
		path = req.Resource
	}
	path = normalizeRequestPath(path, mcpPath)
	if path == "" {
		path = "/"
	}

	target := &url.URL{Scheme: "https", Host: "netlify.local", Path: path, RawQuery: buildRawQuery(req.QueryStringParameters, req.MultiValueQueryStringParameters)}
	body, err := requestBody(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyHeaders(httpReq.Header, req.Headers, req.MultiValueHeaders)
	return httpReq, nil
}

func normalizeRequestPath(path, mcpPath string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return mcpPath
	}
	functionPrefix := "/.netlify/functions/mcp"
	if path == functionPrefix {
		return mcpPath
	}
	if strings.HasPrefix(path, functionPrefix+"/") {
		suffix := strings.TrimPrefix(path, functionPrefix)
		if suffix == "" || suffix == "/" {
			return mcpPath
		}
		return suffix
	}
	return path
}

func buildRawQuery(single map[string]string, multi map[string][]string) string {
	values := url.Values{}
	if len(multi) > 0 {
		for key, arr := range multi {
			for _, v := range arr {
				values.Add(key, v)
			}
		}
		return values.Encode()
	}
	for key, v := range single {
		values.Set(key, v)
	}
	return values.Encode()
}

func requestBody(req events.APIGatewayProxyRequest) ([]byte, error) {
	if req.Body == "" {
		return nil, nil
	}
	if !req.IsBase64Encoded {
		return []byte(req.Body), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(req.Body)
	if err != nil {
		return nil, fmt.Errorf("请求体 Base64 解码失败: %w", err)
	}
	return decoded, nil
}

func copyHeaders(dst http.Header, single map[string]string, multi map[string][]string) {
	for key, arr := range multi {
		for _, v := range arr {
			dst.Add(key, v)
		}
	}
	for key, v := range single {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if len(multi[key]) > 0 {
			continue
		}
		dst.Set(key, v)
	}
}

func textToolResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
