package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchInput struct {
	Query      string  `json:"query" jsonschema:"搜索关键词"`
	PageNo     *int    `json:"pageno,omitempty" jsonschema:"页码，从1开始"`
	TimeRange  *string `json:"time_range,omitempty" jsonschema:"时间范围: day/month/year"`
	Language   *string `json:"language,omitempty" jsonschema:"语言代码，默认 all"`
	SafeSearch *int    `json:"safesearch,omitempty" jsonschema:"安全搜索级别 0/1/2"`
}

type searxResponse struct {
	Results []struct {
		Title   string  `json:"title"`
		Content string  `json:"content"`
		URL     string  `json:"url"`
		Score   float64 `json:"score"`
	} `json:"results"`
}

func (s *Service) handleSearchTool(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, map[string]any, error) {
	result, err := s.performSearch(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	return textToolResult(result), map[string]any{"ok": true}, nil
}

func (s *Service) performSearch(ctx context.Context, in SearchInput) (string, error) {
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return "", fmt.Errorf("query 不能为空")
	}

	endpoint := *s.searxURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/search"

	params := endpoint.Query()
	params.Set("q", q)
	params.Set("format", "json")

	page := 1
	if in.PageNo != nil && *in.PageNo > 0 {
		page = *in.PageNo
	}
	params.Set("pageno", strconv.Itoa(page))

	if in.TimeRange != nil {
		t := strings.ToLower(strings.TrimSpace(*in.TimeRange))
		if t != "" {
			switch t {
			case "day", "month", "year":
				params.Set("time_range", t)
			default:
				return "", fmt.Errorf("time_range 仅支持 day/month/year")
			}
		}
	}

	lang := "all"
	if in.Language != nil && strings.TrimSpace(*in.Language) != "" {
		lang = strings.TrimSpace(*in.Language)
	}
	if !strings.EqualFold(lang, "all") {
		params.Set("language", lang)
	}

	if in.SafeSearch != nil {
		level := *in.SafeSearch
		if level < 0 || level > 2 {
			return "", fmt.Errorf("safesearch 仅支持 0/1/2")
		}
		params.Set("safesearch", strconv.Itoa(level))
	}

	endpoint.RawQuery = params.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, s.cfg.SearchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	if s.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", s.cfg.UserAgent)
	}
	if s.cfg.AuthUsername != "" {
		req.SetBasicAuth(s.cfg.AuthUsername, s.cfg.AuthPassword)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 SearXNG 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("SearXNG 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded searxResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("解析 SearXNG 响应失败: %w", err)
	}

	if len(decoded.Results) == 0 {
		return fmt.Sprintf("🔍 没有检索到结果：%q", q), nil
	}

	var b strings.Builder
	for i, item := range decoded.Results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Title: ")
		b.WriteString(strings.TrimSpace(item.Title))
		b.WriteString("\nDescription: ")
		b.WriteString(strings.TrimSpace(item.Content))
		b.WriteString("\nURL: ")
		b.WriteString(strings.TrimSpace(item.URL))
		b.WriteString("\nRelevance Score: ")
		b.WriteString(strconv.FormatFloat(item.Score, 'f', 3, 64))
	}

	return b.String(), nil
}

func mustJoin(base *url.URL, p string) string {
	copyURL := *base
	copyURL.Path = strings.TrimRight(copyURL.Path, "/") + p
	return copyURL.String()
}
