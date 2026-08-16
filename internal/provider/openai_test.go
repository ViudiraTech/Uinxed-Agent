package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func collect(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var out []Event
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestChatCompletionsSSEContentReasoningToolAndUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization=%q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`{"model":"test","choices":[{"delta":{"reasoning_content":"think "}}]}`,
			`{"model":"test","choices":[{"delta":{"content":"hello "}}]}`,
			`{"model":"test","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_","arguments":"{\\\"path\\\":"}}]}}]}`,
			`{"model":"test","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"file","arguments":"\\\"README.md\\\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
			`[DONE]`,
		}
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			if x, ok := w.(http.Flusher); ok {
				x.Flush()
			}
		}
	}))
	defer srv.Close()
	p := NewOpenAICompatible(config.Provider{ID: "test", BaseURL: srv.URL + "/v1", SupportsThinking: true}, func() (string, error) { return "test-key", nil })
	ch, err := p.Stream(context.Background(), Request{Model: "test", Messages: []domain.Message{{Role: domain.RoleUser, Content: "go"}}})
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	var content, reasoning string
	var calls []domain.ToolCall
	var usage domain.Usage
	done := false
	for _, ev := range events {
		switch ev.Kind {
		case EventContent:
			content += ev.Text
		case EventReasoning:
			reasoning += ev.Text
		case EventToolCall:
			calls = append(calls, ev.ToolCalls...)
		case EventUsage:
			if ev.Usage.TotalTokens > 0 {
				usage = ev.Usage
			}
		case EventDone:
			done = true
		case EventError:
			t.Fatal(ev.Err)
		}
	}
	if content != "hello " || reasoning != "think " {
		t.Fatalf("content=%q reasoning=%q", content, reasoning)
	}
	if len(calls) != 2 { // provider emits deltas; runtime accumulator merges by Index.
		t.Fatalf("tool deltas=%d events=%#v", len(calls), events)
	}
	if calls[0].Index != 0 || calls[1].Index != 0 {
		t.Fatalf("tool index lost: %#v", calls)
	}
	if usage.TotalTokens != 15 || !done {
		t.Fatalf("usage=%+v done=%v", usage, done)
	}
}

func TestResponsesSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, f := range []string{
			`{"type":"response.reasoning_summary_text.delta","delta":"r"}`,
			`{"type":"response.output_text.delta","delta":"ok"}`,
			`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc1","call_id":"call_1","name":"calc"}}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc1","delta":"{\\\"expr\\\":\\\"1+1\\\"}"}`,
			`{"type":"response.completed","response":{"model":"r-model","usage":{"input_tokens":3,"output_tokens":4}}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", f)
		}
	}))
	defer srv.Close()
	p := NewOpenAICompatible(config.Provider{ID: "openai", BaseURL: srv.URL + "/v1", WireAPI: "responses"}, func() (string, error) { return "", nil })
	ch, err := p.Stream(context.Background(), Request{Model: "r-model", Messages: []domain.Message{{Role: domain.RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch)
	var text, reason string
	var call *domain.ToolCall
	var total int64
	for _, e := range evs {
		if e.Kind == EventError {
			t.Fatal(e.Err)
		}
		if e.Kind == EventContent {
			text += e.Text
		}
		if e.Kind == EventReasoning {
			reason += e.Text
		}
		if e.Kind == EventToolCall && len(e.ToolCalls) > 0 {
			c := e.ToolCalls[0]
			call = &c
		}
		if e.Kind == EventUsage {
			total = e.Usage.TotalTokens
		}
	}
	if text != "ok" || reason != "r" || call == nil || call.ID != "call_1" || call.Function.Name != "calc" || !strings.Contains(call.Function.Arguments, "1+1") || total != 7 {
		t.Fatalf("events=%#v", evs)
	}
}

func TestCancellationStopsStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	p := NewOpenAICompatible(config.Provider{ID: "test", BaseURL: srv.URL}, func() (string, error) { return "", nil })
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Stream(ctx, Request{Model: "x", Messages: []domain.Message{{Role: domain.RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		if ev.Kind != EventContent {
			t.Fatalf("first=%#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no first event")
	}
	cancel()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
}

func TestResponsesInputPreservesFunctionCallID(t *testing.T) {
	in := []domain.Message{
		{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "call_42", Type: "function", Function: domain.ToolCallFunction{Name: "calc", Arguments: `{"expr":"2+2"}`}}}},
		{Role: domain.RoleTool, ToolCallID: "call_42", Content: `{"result":4}`},
	}
	got := responsesInput(in)
	if len(got) != 2 {
		t.Fatalf("items=%d %#v", len(got), got)
	}
	call, ok := got[0].(map[string]any)
	if !ok || call["type"] != "function_call" || call["call_id"] != "call_42" {
		t.Fatalf("function call=%#v", got[0])
	}
	output, ok := got[1].(map[string]any)
	if !ok || output["type"] != "function_call_output" || output["call_id"] != "call_42" {
		t.Fatalf("function output=%#v", got[1])
	}
}

func TestProviderKeyErrorIsNotSilentlyIgnored(t *testing.T) {
	p := NewOpenAICompatible(config.Provider{ID: "test", BaseURL: "http://127.0.0.1:1"}, func() (string, error) {
		return "", fmt.Errorf("decrypt failed")
	})
	ch, err := p.Stream(context.Background(), Request{Model: "x", Messages: []domain.Message{{Role: domain.RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch)
	if len(evs) != 1 || evs[0].Kind != EventError || evs[0].Err == nil || !strings.Contains(evs[0].Err.Error(), "decrypt failed") {
		t.Fatalf("events=%#v", evs)
	}
}

func TestProfilePropagatesKeyError(t *testing.T) {
	p := NewOpenAICompatible(config.Provider{ID: "ux-gateway", BaseURL: "http://127.0.0.1:1"}, func() (string, error) {
		return "", fmt.Errorf("decrypt profile key failed")
	})
	_, err := p.Profile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "decrypt profile key failed") {
		t.Fatalf("err=%v", err)
	}
}
