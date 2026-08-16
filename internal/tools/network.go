package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var toolHTTP = &http.Client{Transport: &http.Transport{
	Proxy:             http.ProxyFromEnvironment,
	DialContext:       (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2: true, MaxIdleConns: 64, MaxIdleConnsPerHost: 8, IdleConnTimeout: 90 * time.Second,
	TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 20 * time.Second,
}}

type FetchURLTool struct{}

func (*FetchURLTool) Name() string { return "fetch_url" }
func (*FetchURLTool) Description() string {
	return "抓取 HTTP/HTTPS 网页正文并转为简化文本，限制响应大小并支持取消。"
}
func (*FetchURLTool) Category() Category { return CategoryNetwork }
func (*FetchURLTool) Schema() map[string]any {
	return obj(map[string]any{"url": strp("完整 http/https URL")}, "url")
}
func (*FetchURLTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		URL string `json:"url"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	u, err := url.Parse(a.URL)
	if err != nil {
		return Result{}, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Result{}, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", "ux-agent/2.0")
	resp, err := toolHTTP.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Result{}, err
	}
	ct := resp.Header.Get("Content-Type")
	text := string(body)
	if strings.Contains(ct, "html") {
		text = htmlToText(text)
	}
	return Result{Content: truncate(text, 64<<10), Metadata: map[string]any{"status": resp.StatusCode, "content_type": ct, "url": resp.Request.URL.String()}}, nil
}

type WebSearchTool struct{}

func (*WebSearchTool) Name() string { return "web_search" }
func (*WebSearchTool) Description() string {
	return "搜索公开网页，DuckDuckGo HTML 优先、Bing 回退；返回标题、URL 与摘要。"
}
func (*WebSearchTool) Category() Category { return CategoryNetwork }
func (*WebSearchTool) Schema() map[string]any {
	return obj(map[string]any{"query": strp("搜索关键词"), "max": nump("返回条数，默认 6，最大 10")}, "query")
}
func (*WebSearchTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Query string `json:"query"`
		Max   int    `json:"max"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	if a.Max <= 0 {
		a.Max = 6
	}
	if a.Max > 10 {
		a.Max = 10
	}
	if r, err := searchDDG(ctx, a.Query, a.Max); err == nil && len(r) > 0 {
		return Result{Content: formatSearch(r), Metadata: map[string]any{"engine": "duckduckgo", "count": len(r)}}, nil
	}
	r, err := searchBing(ctx, a.Query, a.Max)
	if err != nil {
		return Result{}, err
	}
	return Result{Content: formatSearch(r), Metadata: map[string]any{"engine": "bing", "count": len(r)}}, nil
}

type searchResult struct{ Title, URL, Snippet string }

var tagRE = regexp.MustCompile(`(?s)<[^>]+>`)
var scriptRE = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
var styleRE = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
var wsRE = regexp.MustCompile(`\s+`)

func htmlToText(s string) string {
	s = scriptRE.ReplaceAllString(s, " ")
	s = styleRE.ReplaceAllString(s, " ")
	s = tagRE.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(wsRE.ReplaceAllString(s, " "))
}
func fetchHTML(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/151 Safari/537.36")
	resp, err := toolHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("search HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}
func searchDDG(ctx context.Context, q string, max int) ([]searchResult, error) {
	page, err := fetchHTML(ctx, "https://html.duckduckgo.com/html/?q="+url.QueryEscape(q))
	if err != nil {
		return nil, err
	}
	// Go's regexp engine intentionally has no lookaround/backreferences. Parse
	// the stable result anchors and snippets independently, then pair them in
	// document order. This avoids regex features that would panic under RE2.
	aRE := regexp.MustCompile(`(?is)<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	sRE := regexp.MustCompile(`(?is)class="result__snippet"[^>]*>(.*?)</(?:a|div)>`)
	anchors := aRE.FindAllStringSubmatch(page, -1)
	snippets := sRE.FindAllStringSubmatch(page, -1)
	var out []searchResult
	for i, m := range anchors {
		if len(m) < 3 {
			continue
		}
		link := html.UnescapeString(m[1])
		if strings.Contains(link, "ad_domain") {
			continue
		}
		if u, err := url.Parse(link); err == nil {
			if v := u.Query().Get("uddg"); v != "" {
				link = v
			}
		}
		snip := ""
		if i < len(snippets) && len(snippets[i]) > 1 {
			snip = htmlToText(snippets[i][1])
		}
		out = append(out, searchResult{htmlToText(m[2]), link, snip})
		if len(out) >= max {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no DuckDuckGo results")
	}
	return out, nil
}
func searchBing(ctx context.Context, q string, max int) ([]searchResult, error) {
	page, err := fetchHTML(ctx, fmt.Sprintf("https://www.bing.com/search?q=%s&count=%d", url.QueryEscape(q), max))
	if err != nil {
		return nil, err
	}
	blockRE := regexp.MustCompile(`(?is)<li class="b_algo".*?</li>`)
	aRE := regexp.MustCompile(`(?is)<h2[^>]*><a[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	pRE := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	var out []searchResult
	for _, b := range blockRE.FindAllString(page, -1) {
		m := aRE.FindStringSubmatch(b)
		if len(m) < 3 {
			continue
		}
		snip := ""
		if sm := pRE.FindStringSubmatch(b); len(sm) > 1 {
			snip = htmlToText(sm[1])
		}
		out = append(out, searchResult{htmlToText(m[2]), html.UnescapeString(m[1]), snip})
		if len(out) >= max {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("search failed: no results")
	}
	return out, nil
}
func formatSearch(rs []searchResult) string {
	var b strings.Builder
	for i, r := range rs {
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return strings.TrimSpace(b.String())
}
