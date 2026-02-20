package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "qoomezhu/mcp-searxng-go"
	serverVersion = "1.1.0"
)

type Config struct {
	SearxngURL          string
	AuthUsername        string
	AuthPassword        string
	UserAgent           string
	ListenAddr          string
	MCPPath             string
	CacheTTL            time.Duration
	FetchTimeout        time.Duration
	SearchTimeout       time.Duration
	MaxBodyBytes        int64
	AllowPrivateAddress bool
	SessionTimeout      time.Duration
	AndroidCompat       bool
}

type Service struct {
	cfg      Config
	searxURL *url.URL
	client   *http.Client
	cache    *URLCache
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

	mcpServer := createMCPServer(svc)
	streamable := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{
			SessionTimeout: cfg.SessionTimeout,
			JSONResponse:   true,
		},
	)

	mcpHandler := http.Handler(streamable)
	if cfg.AndroidCompat {
		mcpHandler = withMobileClientCompatibility(mcpHandler)
	}
	mcpHandler = withCORS(mcpHandler)

	mux := http.NewServeMux()
	mux.Handle(cfg.MCPPath, mcpHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":            "ok",
			"server":            serverName,
			"version":           serverVersion,
			"transport":         "streamable-http",
			"endpoint":          cfg.MCPPath,
			"androidCompatible": cfg.AndroidCompat,
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":                 serverName,
			"version":              serverVersion,
			"protocol":             "streamable-http",
			"studioCompatible":     true,
			"androidLLMCompatible": cfg.AndroidCompat,
			"mcpEndpoint":          cfg.MCPPath,
			"healthEndpoint":       "/health",
		})
	})

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("%s v%s 已启动", serverName, serverVersion)
	log.Printf("监听地址: %s", cfg.ListenAddr)
	log.Printf("MCP 端点: %s", cfg.MCPPath)
	log.Printf("SearXNG: %s", cfg.SearxngURL)
	if cfg.AndroidCompat {
		log.Printf("已启用 Android LLM 兼容模式")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("收到退出信号: %s", sig.String())
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("优雅关闭失败: %v", err)
	}
}

func createMCPServer(svc *Service) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, &mcp.ServerOptions{
		Logger: slog.Default(),
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "searxng_web_search",
		Description: "Use SearXNG for web search with optional page/time/language/safesearch filters.",
	}, svc.handleSearchTool)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "web_url_read",
		Description: "Fetch URL content and convert HTML to markdown with heading/section/paragraph pagination options.",
	}, svc.handleReadTool)

	return srv
}

func newService(cfg Config) (*Service, error) {
	base, err := url.Parse(cfg.SearxngURL)
	if err != nil {
		return nil, fmt.Errorf("SEARXNG_URL 格式无效: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("SEARXNG_URL 只支持 http/https")
	}

	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	client := &http.Client{Transport: transport}

	return &Service{
		cfg:      cfg,
		searxURL: base,
		client:   client,
		cache:    NewURLCache(cfg.CacheTTL),
	}, nil
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
		SessionTimeout:      time.Duration(getEnvInt("MCP_SESSION_TIMEOUT_SECONDS", 1800)) * time.Second,
		AndroidCompat:       getEnvBool("MCP_ANDROID_COMPAT", true),
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

	port := strings.TrimSpace(os.Getenv("MCP_HTTP_PORT"))
	if port == "" {
		port = strings.TrimSpace(os.Getenv("PORT"))
	}
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return Config{}, fmt.Errorf("MCP_HTTP_PORT 非法: %s", port)
	}
	cfg.ListenAddr = ":" + port

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
	allowHeaders := strings.Join([]string{
		"Content-Type",
		"Accept",
		"Authorization",
		"Mcp-Session-Id",
		"Mcp-Protocol-Version",
		"Last-Event-ID",
		"mcp-session-id",
		"mcp-protocol-version",
		"last-event-id",
	}, ",")

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

	required := []string{"application/json", "text/event-stream"}
	for _, want := range required {
		if !containsMime(accept, want) {
			accept += ", " + want
		}
	}
	h.Set("Accept", accept)
}

func containsMime(accept, want string) bool {
	parts := strings.Split(accept, ",")
	for _, item := range parts {
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
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).String())
	})
}

func textToolResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}
