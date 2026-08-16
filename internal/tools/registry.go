package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

type Registry struct {
	mu        sync.RWMutex
	tools     map[string]Tool
	scheduler *Scheduler
}

func NewRegistry(s *Scheduler) *Registry {
	if s == nil {
		s = NewScheduler(DefaultLimits())
	}
	return &Registry{tools: map[string]Tool{}, scheduler: s}
}

func DefaultRegistry() *Registry {
	r := NewRegistry(nil)
	for _, t := range []Tool{
		&BashTool{}, &ReadFileTool{}, &WriteFileTool{}, &EditFileTool{}, &ListDirTool{}, &GrepTool{}, &GlobTool{},
		&FetchURLTool{}, &WebSearchTool{}, &SkillTool{}, &CurrentTimeTool{}, &CalcTool{},
		&DelegateTool{}, &TodoWriteTool{}, &TodoUpdateTool{},
	} {
		r.Register(t)
	}
	return r
}

func (r *Registry) Register(t Tool) { r.mu.Lock(); defer r.mu.Unlock(); r.tools[t.Name()] = t }
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) Definitions(allow func(string) bool) []providerDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []providerDef
	for _, t := range r.tools {
		if allow != nil && !allow(t.Name()) {
			continue
		}
		out = append(out, providerDef{t.Name(), Definition(t)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type providerDef struct {
	Name       string
	Definition any
}

func (r *Registry) ProviderDefinitions(allow func(string) bool) []provider.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []provider.ToolDefinition
	for _, t := range r.tools {
		if allow == nil || allow(t.Name()) {
			out = append(out, Definition(t))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Function.Name < out[j].Function.Name })
	return out
}

func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage, env ExecutionContext) (Result, error) {
	t, ok := r.Get(name)
	if !ok {
		return Result{}, fmt.Errorf("unknown tool %q", name)
	}
	var res Result
	err := r.scheduler.Do(ctx, t.Category(), func(ctx context.Context) error {
		var e error
		res, e = t.Execute(ctx, input, env)
		return e
	})
	return res, err
}
