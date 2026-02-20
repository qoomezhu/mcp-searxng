package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type URLReadInput struct {
	URL            string  `json:"url" jsonschema:"要读取的 URL"`
	StartChar      *int    `json:"startChar,omitempty" jsonschema:"起始字符位置"`
	MaxLength      *int    `json:"maxLength,omitempty" jsonschema:"最多返回字符数"`
	Section        *string `json:"section,omitempty" jsonschema:"按标题提取分节"`
	ParagraphRange *string `json:"paragraphRange,omitempty" jsonschema:"段落范围，如 1-5 / 3 / 10-"`
	ReadHeadings   *bool   `json:"readHeadings,omitempty" jsonschema:"仅返回标题列表"`
}

func (s *Service) handleReadTool(ctx context.Context, _ *mcp.CallToolRequest, in URLReadInput) (*mcp.CallToolResult, map[string]any, error) {
	result, err := s.readURL(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	return textToolResult(result), map[string]any{"ok": true}, nil
}

func (s *Service) readURL(ctx context.Context, in URLReadInput) (string, error) {
	target := strings.TrimSpace(in.URL)
	if target == "" {
		return "", fmt.Errorf("url 不能为空")
	}

	normalizedURL, err := ValidateOutboundURL(target, s.cfg.AllowPrivateAddress)
	if err != nil {
		return "", err
	}
	target = normalizedURL.String()

	md, ok := s.cache.Get(target)
	if !ok {
		md, err = s.fetchAndConvert(ctx, target)
		if err != nil {
			return "", err
		}
		s.cache.Set(target, md)
	}

	return applyReadOptions(md, in)
}

func (s *Service) fetchAndConvert(ctx context.Context, target string) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, s.cfg.FetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	if s.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", s.cfg.UserAgent)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("拉取 URL 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("目标网站返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	limited := io.LimitReader(resp.Body, s.cfg.MaxBodyBytes+1)
	htmlBytes, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("读取网页内容失败: %w", err)
	}
	if int64(len(htmlBytes)) > s.cfg.MaxBodyBytes {
		return "", fmt.Errorf("网页内容过大，超过限制 %d bytes", s.cfg.MaxBodyBytes)
	}

	markdown, err := htmltomarkdown.ConvertString(string(htmlBytes))
	if err != nil {
		return "", fmt.Errorf("HTML 转 Markdown 失败: %w", err)
	}
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return "", fmt.Errorf("网页内容为空或无法转换")
	}

	return markdown, nil
}

func applyReadOptions(markdown string, in URLReadInput) (string, error) {
	result := markdown

	if in.ReadHeadings != nil && *in.ReadHeadings {
		return extractHeadings(result), nil
	}

	if in.Section != nil {
		section := strings.TrimSpace(*in.Section)
		if section != "" {
			extracted := extractSection(result, section)
			if extracted == "" {
				return "", fmt.Errorf("未找到 section: %s", section)
			}
			result = extracted
		}
	}

	if in.ParagraphRange != nil {
		r := strings.TrimSpace(*in.ParagraphRange)
		if r != "" {
			extracted, err := extractParagraphRange(result, r)
			if err != nil {
				return "", err
			}
			result = extracted
		}
	}

	start := 0
	if in.StartChar != nil {
		if *in.StartChar < 0 {
			return "", fmt.Errorf("startChar 不能为负数")
		}
		start = *in.StartChar
	}

	maxLen := 0
	if in.MaxLength != nil {
		if *in.MaxLength <= 0 {
			return "", fmt.Errorf("maxLength 必须 > 0")
		}
		maxLen = *in.MaxLength
	}

	result = applyCharacterWindow(result, start, maxLen)
	return result, nil
}

func applyCharacterWindow(input string, start, maxLen int) string {
	runes := []rune(input)
	if start >= len(runes) {
		return ""
	}
	if start < 0 {
		start = 0
	}
	end := len(runes)
	if maxLen > 0 && start+maxLen < end {
		end = start + maxLen
	}
	return string(runes[start:end])
}

func extractSection(markdown, heading string) string {
	lines := strings.Split(markdown, "\n")
	pattern := regexp.MustCompile(`(?i)^#{1,6}\s*.*` + regexp.QuoteMeta(heading) + `.*$`)

	start := -1
	level := 0
	for i, line := range lines {
		if pattern.MatchString(line) {
			start = i
			level = headingLevel(line)
			break
		}
	}
	if start < 0 {
		return ""
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if l := headingLevel(lines[i]); l > 0 && l <= level {
			end = i
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

func headingLevel(line string) int {
	trimmed := strings.TrimSpace(line)
	count := 0
	for _, ch := range trimmed {
		if ch == '#' {
			count++
			continue
		}
		break
	}
	if count == 0 || count > 6 {
		return 0
	}
	if len(trimmed) > count && trimmed[count] != ' ' {
		return 0
	}
	return count
}

func extractParagraphRange(markdown, paragraphRange string) (string, error) {
	paragraphs := splitParagraphs(markdown)
	if len(paragraphs) == 0 {
		return "", fmt.Errorf("内容为空")
	}

	matches := regexp.MustCompile(`^(\d+)(?:-(\d*))?$`).FindStringSubmatch(paragraphRange)
	if len(matches) == 0 {
		return "", fmt.Errorf("paragraphRange 格式非法: %s", paragraphRange)
	}

	startIndex, _ := strconv.Atoi(matches[1])
	startIndex--
	if startIndex < 0 || startIndex >= len(paragraphs) {
		return "", fmt.Errorf("paragraphRange 起始位置越界")
	}

	if len(matches) < 3 {
		return paragraphs[startIndex], nil
	}

	endToken := matches[2]
	if endToken == "" {
		return strings.Join(paragraphs[startIndex:], "\n\n"), nil
	}

	endIndex, _ := strconv.Atoi(endToken)
	if endIndex < startIndex+1 {
		return "", fmt.Errorf("paragraphRange 结束位置非法")
	}
	if endIndex > len(paragraphs) {
		endIndex = len(paragraphs)
	}
	return strings.Join(paragraphs[startIndex:endIndex], "\n\n"), nil
}

func splitParagraphs(markdown string) []string {
	parts := regexp.MustCompile(`\n\s*\n`).Split(markdown, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func extractHeadings(markdown string) string {
	lines := strings.Split(markdown, "\n")
	headings := make([]string, 0)
	for _, line := range lines {
		if headingLevel(line) > 0 {
			headings = append(headings, strings.TrimSpace(line))
		}
	}
	if len(headings) == 0 {
		return "No headings found in the content."
	}
	return strings.Join(headings, "\n")
}
