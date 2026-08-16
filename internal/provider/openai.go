package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

type OpenAICompatible struct {
	mu     sync.RWMutex
	cfg    config.Provider
	key    func() (string, error)
	client *http.Client
}

func NewOpenAICompatible(cfg config.Provider, key func() (string, error)) *OpenAICompatible {
	tr := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DialContext:       (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true,
		MaxIdleConns:      100, MaxIdleConnsPerHost: 16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	return &OpenAICompatible{cfg: cfg, key: key, client: &http.Client{Transport: tr}}
}

func (p *OpenAICompatible) Update(cfg config.Provider) { p.mu.Lock(); p.cfg = cfg; p.mu.Unlock() }
func (p *OpenAICompatible) Config() config.Provider    { p.mu.RLock(); defer p.mu.RUnlock(); return p.cfg }

func (p *OpenAICompatible) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	cfg := p.Config()
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("provider base URL is empty")
	}
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		var err error
		if strings.EqualFold(cfg.WireAPI, "responses") {
			err = p.streamResponses(ctx, cfg, req, ch)
		} else {
			err = p.streamChat(ctx, cfg, req, ch)
		}
		if err != nil {
			select {
			case ch <- Event{Kind: EventError, Err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

func (p *OpenAICompatible) doWithRetry(ctx context.Context, build func() (io.Reader, string, error)) (*http.Response, error) {
	cfg := p.Config()
	key, err := p.key()
	if err != nil {
		return nil, fmt.Errorf("load provider key: %w", err)
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		body, endpoint, err := build()
		if err != nil {
			return nil, err
		}
		rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		req, err := http.NewRequestWithContext(rctx, http.MethodPost, endpoint, body)
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream, application/json")
		req.Header.Set("User-Agent", "ux-agent/2.0")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}
		resp, err := p.client.Do(req)
		if err == nil && (resp.StatusCode < 500 && resp.StatusCode != 429) {
			// Body owns the request context. Cancel when body closes.
			resp.Body = &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
			return resp, nil
		}
		if resp != nil {
			// Read retry/error metadata before cancelling the request context;
			// cancelling first can abort the response body and hide the upstream error.
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			retry := retryAfter(resp.Header.Get("Retry-After"))
			_ = resp.Body.Close()
			cancel()
			last = &APIError{Status: resp.StatusCode, Message: extractError(raw)}
			if resp.StatusCode != 429 && resp.StatusCode < 500 {
				return nil, last
			}
			if retry > 0 {
				if err := sleepCtx(ctx, retry); err != nil {
					return nil, err
				}
				continue
			}
		} else {
			cancel()
			last = err
		}
		if attempt < 2 {
			d := time.Duration(250*(1<<attempt))*time.Millisecond + time.Duration(rand.IntN(150))*time.Millisecond
			if err := sleepCtx(ctx, d); err != nil {
				return nil, err
			}
		}
	}
	return nil, last
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error { err := c.ReadCloser.Close(); c.cancel(); return err }

type APIError struct {
	Status  int
	Type    string
	Message string
}

func (e *APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("provider error (%d, %s): %s", e.Status, e.Type, e.Message)
	}
	return fmt.Sprintf("provider error (%d): %s", e.Status, e.Message)
}

func (p *OpenAICompatible) streamChat(ctx context.Context, cfg config.Provider, req Request, out chan<- Event) error {
	payload := map[string]any{"model": req.Model, "messages": chatMessages(req.Messages, cfg.SupportsThinking), "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
	}
	if cfg.ID == "deepseek" && req.Thinking {
		payload["thinking"] = map[string]any{"type": "enabled"}
	}
	if cfg.SupportsEffort && req.Effort != "" {
		e := req.Effort
		if e == "supercode" {
			e = "max"
		}
		payload["reasoning_effort"] = e
	}
	build := func() (io.Reader, string, error) {
		raw, err := json.Marshal(payload)
		return bytes.NewReader(raw), strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions", err
	}
	resp, err := p.doWithRetry(ctx, build)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
		return parseAPIError(resp.StatusCode, raw)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		return p.parseNonStreamChat(ctx, resp, out)
	}
	return parseSSE(ctx, resp.Body, func(data []byte) error {
		if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
			return emit(ctx, out, Event{Kind: EventDone, FinishReason: "stop"})
		}
		var packet chatPacket
		if err := json.Unmarshal(data, &packet); err != nil {
			return nil
		} // tolerate malformed/unknown frames
		if packet.Error != nil {
			return &APIError{Status: resp.StatusCode, Type: packet.Error.Type, Message: packet.Error.Message}
		}
		if packet.Model != "" {
			_ = emit(ctx, out, Event{Kind: EventUsage, Model: packet.Model})
		}
		if packet.Usage != nil {
			if err := emit(ctx, out, Event{Kind: EventUsage, Usage: normalizeUsage(*packet.Usage), Model: packet.Model}); err != nil {
				return err
			}
		}
		if len(packet.Choices) == 0 {
			return nil
		}
		c := packet.Choices[0]
		if c.Delta.Content != "" {
			if err := emit(ctx, out, Event{Kind: EventContent, Text: c.Delta.Content, Model: packet.Model}); err != nil {
				return err
			}
		}
		if c.Delta.Reasoning != "" {
			if err := emit(ctx, out, Event{Kind: EventReasoning, Text: c.Delta.Reasoning, Model: packet.Model}); err != nil {
				return err
			}
		}
		if len(c.Delta.ToolCalls) > 0 {
			calls := make([]domain.ToolCall, 0, len(c.Delta.ToolCalls))
			for _, tc := range c.Delta.ToolCalls {
				calls = append(calls, domain.ToolCall{Index: tc.Index, ID: tc.ID, Type: "function", Function: domain.ToolCallFunction{Name: tc.Function.Name, Arguments: tc.Function.Arguments}})
			}
			if err := emit(ctx, out, Event{Kind: EventToolCall, ToolCalls: calls, Model: packet.Model}); err != nil {
				return err
			}
		}
		if c.FinishReason != "" {
			return emit(ctx, out, Event{Kind: EventDone, FinishReason: c.FinishReason, Model: packet.Model})
		}
		return nil
	})
}

type chatPacket struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning_content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		Message struct {
			Content   string            `json:"content"`
			Reasoning string            `json:"reasoning_content"`
			ToolCalls []domain.ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}
type wireUsage struct {
	Prompt     int64 `json:"prompt_tokens"`
	Completion int64 `json:"completion_tokens"`
	Input      int64 `json:"input_tokens"`
	Output     int64 `json:"output_tokens"`
	Total      int64 `json:"total_tokens"`
}

func normalizeUsage(u wireUsage) domain.Usage {
	in, out := u.Prompt, u.Completion
	if in == 0 {
		in = u.Input
	}
	if out == 0 {
		out = u.Output
	}
	total := u.Total
	if total == 0 {
		total = in + out
	}
	return domain.Usage{InputTokens: in, OutputTokens: out, TotalTokens: total}
}

func (p *OpenAICompatible) parseNonStreamChat(ctx context.Context, resp *http.Response, out chan<- Event) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	var packet chatPacket
	if err := json.Unmarshal(raw, &packet); err != nil {
		return fmt.Errorf("invalid provider JSON: %w", err)
	}
	if packet.Error != nil {
		return &APIError{Status: resp.StatusCode, Type: packet.Error.Type, Message: packet.Error.Message}
	}
	if len(packet.Choices) > 0 {
		m := packet.Choices[0].Message
		if m.Content != "" {
			if err := emit(ctx, out, Event{Kind: EventContent, Text: m.Content, Model: packet.Model}); err != nil {
				return err
			}
		}
		if m.Reasoning != "" {
			if err := emit(ctx, out, Event{Kind: EventReasoning, Text: m.Reasoning, Model: packet.Model}); err != nil {
				return err
			}
		}
		if len(m.ToolCalls) > 0 {
			if err := emit(ctx, out, Event{Kind: EventToolCall, ToolCalls: m.ToolCalls, Model: packet.Model}); err != nil {
				return err
			}
		}
	}
	if packet.Usage != nil {
		if err := emit(ctx, out, Event{Kind: EventUsage, Usage: normalizeUsage(*packet.Usage), Model: packet.Model}); err != nil {
			return err
		}
	}
	if len(packet.Choices) > 0 {
		return emit(ctx, out, Event{Kind: EventDone, FinishReason: packet.Choices[0].FinishReason, Model: packet.Model})
	}
	return nil
}

func chatMessages(in []domain.Message, keepReasoning bool) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, m := range in {
		x := map[string]any{"role": string(m.Role)}
		if m.Content != "" || m.Role == domain.RoleUser || m.Role == domain.RoleSystem {
			x["content"] = m.Content
		}
		if keepReasoning && m.ReasoningContent != "" {
			x["reasoning_content"] = m.ReasoningContent
		}
		if len(m.ToolCalls) > 0 {
			x["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			x["tool_call_id"] = m.ToolCallID
		}
		if m.Name != "" {
			x["name"] = m.Name
		}
		out = append(out, x)
	}
	return out
}

func (p *OpenAICompatible) streamResponses(ctx context.Context, cfg config.Provider, req Request, out chan<- Event) error {
	payload := map[string]any{"model": req.Model, "input": responsesInput(req.Messages), "stream": true, "store": false}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{"type": "function", "name": t.Function.Name, "description": t.Function.Description, "parameters": t.Function.Parameters})
		}
		payload["tools"] = tools
	}
	if req.Effort != "" {
		e := req.Effort
		if e == "supercode" {
			e = "max"
		}
		payload["reasoning"] = map[string]any{"effort": e}
	}
	build := func() (io.Reader, string, error) {
		raw, err := json.Marshal(payload)
		return bytes.NewReader(raw), strings.TrimRight(cfg.BaseURL, "/") + "/responses", err
	}
	resp, err := p.doWithRetry(ctx, build)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
		return parseAPIError(resp.StatusCode, raw)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return p.parseNonStreamResponses(ctx, resp, out)
	}
	// Responses streaming uses item_id for argument delta routing, while
	// function_call_output must use call_id. Keep both identities separate.
	acc := map[string]*domain.ToolCall{} // item_id -> accumulated call
	order := make([]string, 0, 4)
	return parseSSE(ctx, resp.Body, func(data []byte) error {
		var ev map[string]any
		if err := json.Unmarshal(data, &ev); err != nil {
			return nil
		}
		typ, _ := ev["type"].(string)
		switch typ {
		case "response.output_text.delta":
			if v, _ := ev["delta"].(string); v != "" {
				return emit(ctx, out, Event{Kind: EventContent, Text: v})
			}
		case "response.reasoning_summary_text.delta", "response.reasoning.delta":
			if v, _ := ev["delta"].(string); v != "" {
				return emit(ctx, out, Event{Kind: EventReasoning, Text: v})
			}
		case "response.output_item.added":
			item, _ := ev["item"].(map[string]any)
			if item == nil || item["type"] != "function_call" {
				return nil
			}
			itemID := str(item["id"])
			callID := str(item["call_id"])
			if callID == "" {
				callID = itemID // tolerate non-standard compatible providers
			}
			if itemID == "" {
				itemID = callID
			}
			if itemID == "" {
				return nil
			}
			if _, exists := acc[itemID]; !exists {
				order = append(order, itemID)
			}
			acc[itemID] = &domain.ToolCall{ID: callID, Type: "function", Function: domain.ToolCallFunction{Name: str(item["name"])}}
		case "response.function_call_arguments.delta":
			itemID := str(ev["item_id"])
			if tc := acc[itemID]; tc != nil {
				tc.Function.Arguments += str(ev["delta"])
			}
		case "response.function_call_arguments.done":
			itemID := str(ev["item_id"])
			if tc := acc[itemID]; tc != nil {
				if v := str(ev["call_id"]); v != "" {
					tc.ID = v
				}
				if v := str(ev["name"]); v != "" {
					tc.Function.Name = v
				}
				tc.Function.Arguments = str(ev["arguments"])
			}
		case "response.completed", "response.incomplete":
			respObj, _ := ev["response"].(map[string]any)
			calls := make([]domain.ToolCall, 0, len(order))
			for _, itemID := range order {
				if tc := acc[itemID]; tc != nil {
					calls = append(calls, *tc)
				}
			}
			if len(calls) > 0 {
				if err := emit(ctx, out, Event{Kind: EventToolCall, ToolCalls: calls}); err != nil {
					return err
				}
			}
			if u, _ := respObj["usage"].(map[string]any); u != nil {
				in := num(u["input_tokens"])
				ou := num(u["output_tokens"])
				_ = emit(ctx, out, Event{Kind: EventUsage, Usage: domain.Usage{InputTokens: in, OutputTokens: ou, TotalTokens: in + ou}})
			}
			finish := "stop"
			if typ == "response.incomplete" {
				finish = "max_tokens"
			}
			return emit(ctx, out, Event{Kind: EventDone, FinishReason: finish, Model: str(respObj["model"])})
		case "error":
			return &APIError{Status: resp.StatusCode, Message: str(ev["message"])}
		}
		return nil
	})
}

func responsesInput(in []domain.Message) []any {
	out := make([]any, 0, len(in)+4)
	for _, m := range in {
		switch m.Role {
		case domain.RoleSystem, domain.RoleUser:
			out = append(out, map[string]any{"role": string(m.Role), "content": m.Content})
		case domain.RoleAssistant:
			if m.Content != "" {
				out = append(out, map[string]any{"role": "assistant", "content": m.Content})
			}
			for _, tc := range m.ToolCalls {
				out = append(out, map[string]any{"type": "function_call", "call_id": tc.ID, "name": tc.Function.Name, "arguments": tc.Function.Arguments})
			}
		case domain.RoleTool:
			out = append(out, map[string]any{"type": "function_call_output", "call_id": m.ToolCallID, "output": m.Content})
		}
	}
	return out
}

func (p *OpenAICompatible) parseNonStreamResponses(ctx context.Context, resp *http.Response, out chan<- Event) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if e, _ := data["error"].(map[string]any); e != nil {
		return &APIError{Status: resp.StatusCode, Message: str(e["message"])}
	}
	output, _ := data["output"].([]any)
	var calls []domain.ToolCall
	for _, v := range output {
		item, _ := v.(map[string]any)
		if item == nil {
			continue
		}
		switch str(item["type"]) {
		case "message":
			parts, _ := item["content"].([]any)
			for _, pv := range parts {
				part, _ := pv.(map[string]any)
				if str(part["type"]) == "output_text" {
					if err := emit(ctx, out, Event{Kind: EventContent, Text: str(part["text"])}); err != nil {
						return err
					}
				}
			}
		case "function_call":
			id := str(item["call_id"])
			if id == "" {
				id = str(item["id"]) // compatibility fallback
			}
			calls = append(calls, domain.ToolCall{ID: id, Type: "function", Function: domain.ToolCallFunction{Name: str(item["name"]), Arguments: str(item["arguments"])}})
		}
	}
	if len(calls) > 0 {
		if err := emit(ctx, out, Event{Kind: EventToolCall, ToolCalls: calls}); err != nil {
			return err
		}
	}
	if u, _ := data["usage"].(map[string]any); u != nil {
		in, ou := num(u["input_tokens"]), num(u["output_tokens"])
		if err := emit(ctx, out, Event{Kind: EventUsage, Usage: domain.Usage{InputTokens: in, OutputTokens: ou, TotalTokens: in + ou}}); err != nil {
			return err
		}
	}
	status := str(data["status"])
	finish := "stop"
	if status == "incomplete" {
		finish = "max_tokens"
	}
	return emit(ctx, out, Event{Kind: EventDone, FinishReason: finish, Model: str(data["model"])})
}

func (p *OpenAICompatible) Models(ctx context.Context) ([]string, error) {
	cfg := p.Config()
	if cfg.ID != "ux-gateway" {
		return append([]string(nil), cfg.Models...), nil
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	base.Path = strings.TrimSuffix(base.Path, "/v1") + "/api/models"
	key, keyErr := p.key()
	if keyErr != nil {
		return nil, fmt.Errorf("load provider key: %w", keyErr)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return append([]string(nil), cfg.Models...), nil
	}
	defer resp.Body.Close()
	if !success(resp.StatusCode) {
		return append([]string(nil), cfg.Models...), nil
	}
	var data struct {
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&data); err != nil {
		return append([]string(nil), cfg.Models...), nil
	}
	var out []string
	for _, m := range data.Models {
		if m.ID != "" {
			out = append(out, m.ID)
		} else if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), cfg.Models...), nil
	}
	return out, nil
}

func (p *OpenAICompatible) CheckKey(ctx context.Context, key string) error {
	cfg := p.Config()
	if cfg.RequiresAuth != nil && !*cfg.RequiresAuth {
		return nil
	}
	req := Request{Model: cfg.DefaultModel, Messages: []domain.Message{{Role: domain.RoleUser, Content: "ping"}}}
	probe := &OpenAICompatible{cfg: cfg, key: func() (string, error) { return key, nil }, client: p.client}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ch, err := probe.Stream(ctx, req)
	if err != nil {
		return err
	}
	for ev := range ch {
		if ev.Kind == EventError {
			return ev.Err
		}
		if ev.Kind == EventDone {
			return nil
		}
	}
	return errors.New("provider returned no completion")
}

func (p *OpenAICompatible) Profile(ctx context.Context) (map[string]any, error) {
	cfg := p.Config()
	if cfg.ID != "ux-gateway" {
		return nil, errors.New("profile/quota is only available for ux-gateway")
	}
	key, err := p.key()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.BaseURL, "/")+"/me", nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !success(resp.StatusCode) {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, parseAPIError(resp.StatusCode, raw)
	}
	var data map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&data); err != nil {
		return nil, err
	}
	if u, ok := data["user"].(map[string]any); ok {
		return u, nil
	}
	return data, nil
}

func parseSSE(ctx context.Context, r io.Reader, handle func([]byte) error) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 64<<10)
	sc.Buffer(buf, 8<<20)
	var data bytes.Buffer
	flush := func() error {
		if data.Len() == 0 {
			return nil
		}
		raw := bytes.TrimSpace(data.Bytes())
		data.Reset()
		return handle(raw)
	}
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := sc.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return flush()
}

func emit(ctx context.Context, out chan<- Event, e Event) error {
	select {
	case out <- e:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func success(s int) bool { return s >= 200 && s < 300 }
func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func num(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case int64:
		return n
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}
func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if sec, err := strconv.Atoi(v); err == nil {
		return time.Duration(sec) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func extractError(raw []byte) string {
	var d map[string]any
	if json.Unmarshal(raw, &d) == nil {
		if e, ok := d["error"].(map[string]any); ok {
			if m := str(e["message"]); m != "" {
				return m
			}
		}
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 1000 {
		s = s[:1000]
	}
	if s == "" {
		s = "empty response"
	}
	return s
}
func parseAPIError(status int, raw []byte) error {
	var d map[string]any
	_ = json.Unmarshal(raw, &d)
	if e, ok := d["error"].(map[string]any); ok {
		return &APIError{Status: status, Type: str(e["type"]), Message: str(e["message"])}
	}
	return &APIError{Status: status, Message: extractError(raw)}
}
