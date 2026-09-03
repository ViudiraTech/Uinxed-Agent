package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/ViudiraTech/Uinxed-Agent/internal/skills"
)

type SkillTool struct{}

func (*SkillTool) Name() string { return "use_skill" }
func (*SkillTool) Description() string {
	return "加载可用 Agent Skill 的完整指令；任务匹配技能时先调用。"
}
func (*SkillTool) Category() Category { return CategoryRead }
func (*SkillTool) Schema() map[string]any {
	return obj(map[string]any{"skill": strp("技能名称")}, "skill")
}
func (*SkillTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Skill string `json:"skill"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	s, ok, err := skills.Get(a.Skill, env.CWD)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, fmt.Errorf("skill %q not found", a.Skill)
	}
	return Result{Content: s.Body, Metadata: map[string]any{"name": s.Name, "description": s.Description, "dir": s.Dir}}, nil
}

type CalcTool struct{}

func (*CalcTool) Name() string { return "calc" }
func (*CalcTool) Description() string {
	return "安全计算 + - * / % 和括号数学表达式，不使用 eval。"
}
func (*CalcTool) Category() Category { return CategoryState }
func (*CalcTool) Schema() map[string]any {
	return obj(map[string]any{"expr": strp("数学表达式")}, "expr")
}
func (*CalcTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Expr string `json:"expr"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	v, err := evalMath(a.Expr)
	if err != nil {
		return Result{}, err
	}
	return Result{Content: strconv.FormatFloat(v, 'g', -1, 64), Metadata: map[string]any{"result": v}}, nil
}

type DelegateTool struct{}

func (*DelegateTool) Name() string { return "delegate" }
func (*DelegateTool) Description() string {
	return "将独立子任务委托给 explorer/general/coding 子 Agent；运行时并发执行并回传结果。"
}
func (*DelegateTool) Category() Category { return CategoryDelegate }
func (*DelegateTool) Schema() map[string]any {
	return obj(map[string]any{"agent": map[string]any{"type": "string", "enum": []string{"explorer", "general", "coding"}}, "task": strp("子任务描述")}, "agent", "task")
}
func (*DelegateTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	if env.Callbacks.Delegate == nil {
		return Result{}, errors.New("delegate unavailable in this runtime")
	}
	return env.Callbacks.Delegate(ctx, raw)
}

type TodoWriteTool struct{}

func (*TodoWriteTool) Name() string { return "todo_write" }
func (*TodoWriteTool) Description() string {
	return "创建或重置完整任务清单，供多步任务进度可视化。"
}
func (*TodoWriteTool) Category() Category { return CategoryState }
func (*TodoWriteTool) Schema() map[string]any {
	return obj(map[string]any{"todos": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"subject": strp("任务描述"), "status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}}}, "required": []string{"subject"}}}}, "todos")
}
func (*TodoWriteTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	if env.Callbacks.TodoWrite == nil {
		return Result{}, errors.New("todo callback unavailable")
	}
	return env.Callbacks.TodoWrite(ctx, raw)
}

type TodoUpdateTool struct{}

func (*TodoUpdateTool) Name() string        { return "todo_update" }
func (*TodoUpdateTool) Description() string { return "按序号或 subject 更新任务状态。" }
func (*TodoUpdateTool) Category() Category  { return CategoryState }
func (*TodoUpdateTool) Schema() map[string]any {
	return obj(map[string]any{"index": nump("任务序号（1 开始）"), "subject": strp("任务描述"), "status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}}, "reason": strp("变更原因")}, "status")
}
func (*TodoUpdateTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	if env.Callbacks.TodoUpdate == nil {
		return Result{}, errors.New("todo callback unavailable")
	}
	return env.Callbacks.TodoUpdate(ctx, raw)
}

type parser struct {
	s []rune
	i int
}

func evalMath(s string) (float64, error) {
	p := &parser{s: []rune(strings.TrimSpace(s))}
	v, err := p.expr()
	if err != nil {
		return 0, err
	}
	p.ws()
	if p.i != len(p.s) {
		return 0, fmt.Errorf("unexpected character %q", p.s[p.i])
	}
	return v, nil
}
func (p *parser) ws() {
	for p.i < len(p.s) && unicode.IsSpace(p.s[p.i]) {
		p.i++
	}
}
func (p *parser) expr() (float64, error) {
	v, e := p.term()
	if e != nil {
		return 0, e
	}
	for {
		p.ws()
		if p.i >= len(p.s) || (p.s[p.i] != '+' && p.s[p.i] != '-') {
			return v, nil
		}
		op := p.s[p.i]
		p.i++
		r, e := p.term()
		if e != nil {
			return 0, e
		}
		if op == '+' {
			v += r
		} else {
			v -= r
		}
	}
}
func (p *parser) term() (float64, error) {
	v, e := p.factor()
	if e != nil {
		return 0, e
	}
	for {
		p.ws()
		if p.i >= len(p.s) || (p.s[p.i] != '*' && p.s[p.i] != '/' && p.s[p.i] != '%') {
			return v, nil
		}
		op := p.s[p.i]
		p.i++
		r, e := p.factor()
		if e != nil {
			return 0, e
		}
		if (op == '/' || op == '%') && r == 0 {
			return 0, errors.New("division by zero")
		}
		switch op {
		case '*':
			v *= r
		case '/':
			v /= r
		case '%':
			v = float64(int64(v) % int64(r))
		}
	}
}
func (p *parser) factor() (float64, error) {
	p.ws()
	if p.i >= len(p.s) {
		return 0, errors.New("unexpected end")
	}
	sign := 1.0
	if p.s[p.i] == '+' || p.s[p.i] == '-' {
		if p.s[p.i] == '-' {
			sign = -1
		}
		p.i++
		p.ws()
	}
	if p.i < len(p.s) && p.s[p.i] == '(' {
		p.i++
		v, e := p.expr()
		if e != nil {
			return 0, e
		}
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != ')' {
			return 0, errors.New("missing )")
		}
		p.i++
		return sign * v, nil
	}
	start := p.i
	dot := false
	for p.i < len(p.s) {
		r := p.s[p.i]
		if r == '.' && !dot {
			dot = true
			p.i++
			continue
		}
		if r < '0' || r > '9' {
			break
		}
		p.i++
	}
	if start == p.i {
		return 0, fmt.Errorf("expected number")
	}
	v, e := strconv.ParseFloat(string(p.s[start:p.i]), 64)
	return sign * v, e
}
