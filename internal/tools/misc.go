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

type PlanWriteTool struct{}

func (*PlanWriteTool) Name() string { return "plan_write" }
func (*PlanWriteTool) Description() string {
	return "在 plan 工作模式中创建或重置结构化实施计划；不修改工作区，结果可用 /plan 查看。"
}
func (*PlanWriteTool) Category() Category { return CategoryState }
func (*PlanWriteTool) Schema() map[string]any {
	return obj(map[string]any{"steps": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"subject": strp("计划步骤"), "details": strp("实现细节"), "status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}}}, "required": []string{"subject"}}}}, "steps")
}
func (*PlanWriteTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	if env.Callbacks.PlanWrite == nil {
		return Result{}, errors.New("plan callback unavailable")
	}
	return env.Callbacks.PlanWrite(ctx, raw)
}

type SwitchModeTool struct{}

func (*SwitchModeTool) Name() string { return "switch_mode" }
func (*SwitchModeTool) Description() string {
	return "提议切换当前会话的工作模式；进入 plan 模式直接生效，退出 plan 必须改用 exit_plan，其余切换需用户确认。子智能体无权切换。"
}
func (*SwitchModeTool) Category() Category { return CategoryState }
func (*SwitchModeTool) Schema() map[string]any {
	return obj(map[string]any{"mode": map[string]any{"type": "string", "enum": []string{"plan", "read-only", "auto-edit", "full-auto"}}, "reason": strp("切换原因")}, "mode")
}
func (*SwitchModeTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	if env.Callbacks.SwitchMode == nil {
		return Result{}, errors.New("mode switch unavailable")
	}
	return env.Callbacks.SwitchMode(ctx, raw)
}

type ExitPlanTool struct{}

func (*ExitPlanTool) Name() string { return "exit_plan" }
func (*ExitPlanTool) Description() string {
	return "计划写完后提交给用户审批；仅 plan 模式可用。用户批准后才会离开 plan 并开始实现，拒绝则继续规划。"
}
func (*ExitPlanTool) Category() Category { return CategoryState }
func (*ExitPlanTool) Schema() map[string]any {
	return obj(map[string]any{
		"summary": strp("给用户看的计划摘要（可选）"),
		"mode":    map[string]any{"type": "string", "enum": []string{"read-only", "auto-edit", "full-auto"}, "description": "用户选择的实现模式；模型无需填写"},
	})
}
func (*ExitPlanTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	if env.Callbacks.ExitPlan == nil {
		return Result{}, errors.New("exit_plan unavailable")
	}
	return env.Callbacks.ExitPlan(ctx, raw)
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
