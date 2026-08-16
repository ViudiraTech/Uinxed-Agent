package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const skillFile = "SKILL.md"

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Skill struct {
	Name        string
	Description string
	Body        string
	Dir         string
	Builtin     bool
}

var projectRoots = []string{
	filepath.Join(".ux-agent", "skills"),
	filepath.Join(".opencode", "skills"),
	filepath.Join(".claude", "skills"),
	filepath.Join(".agents", "skills"),
}
var globalRoots = []string{
	filepath.Join(".config", "ux-agent", "skills"),
	filepath.Join(".config", "opencode", "skills"),
	filepath.Join(".claude", "skills"),
	filepath.Join(".agents", "skills"),
}

func Parse(raw string) (Skill, error) {
	s := Skill{Body: strings.TrimSpace(raw)}
	if !strings.HasPrefix(raw, "---\n") && !strings.HasPrefix(raw, "---\r\n") {
		return s, nil
	}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return s, fmt.Errorf("unterminated frontmatter")
	}
	for _, line := range lines[1:end] {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "name":
			s.Name = v
		case "description":
			s.Description = v
		}
	}
	s.Body = strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	return s, nil
}

func roots(cwd string) ([]string, error) {
	var out []string
	if cwd != "" {
		for _, rel := range projectRoots {
			out = append(out, filepath.Join(cwd, rel))
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	for _, rel := range globalRoots {
		out = append(out, filepath.Join(home, rel))
	}
	return out, nil
}

func List(cwd string) ([]Skill, error) {
	rs, err := roots(cwd)
	if err != nil {
		return nil, err
	}
	seen := map[string]Skill{}
	for _, root := range rs {
		ents, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				info, err := e.Info()
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					continue
				}
			}
			dir := filepath.Join(root, e.Name())
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, skillFile))
			if err != nil {
				continue
			}
			sk, err := Parse(string(raw))
			if err != nil {
				continue
			}
			if sk.Name == "" {
				sk.Name = e.Name()
			}
			if !namePattern.MatchString(sk.Name) || len(sk.Name) > 64 || len(sk.Description) > 1024 {
				continue
			}
			if sk.Name != e.Name() {
				continue
			}
			if _, ok := seen[sk.Name]; ok {
				continue
			}
			if len(sk.Body) > 20000 {
				sk.Body = sk.Body[:20000]
			}
			sk.Dir = dir
			seen[sk.Name] = sk
		}
	}
	for _, b := range builtins() {
		if _, ok := seen[b.Name]; !ok {
			seen[b.Name] = b
		}
	}
	out := make([]Skill, 0, len(seen))
	for _, s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func Get(name, cwd string) (Skill, bool, error) {
	all, err := List(cwd)
	if err != nil {
		return Skill{}, false, err
	}
	for _, s := range all {
		if s.Name == name {
			return s, true, nil
		}
	}
	return Skill{}, false, nil
}

func PromptBlock(cwd string) string {
	all, err := List(cwd)
	if err != nil || len(all) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[可用技能]\n你拥有以下技能(遵循 Agent Skills 开放标准),任务匹配时用 use_skill 工具加载完整指令后再执行:\n")
	for _, s := range all {
		b.WriteString("- ")
		b.WriteString(s.Name)
		b.WriteString(": ")
		if s.Description == "" {
			b.WriteString("(无描述)")
		} else {
			b.WriteString(s.Description)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func ValidateDir(dir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, skillFile))
	if err != nil {
		return err
	}
	s, err := Parse(string(raw))
	if err != nil {
		return err
	}
	if s.Name == "" {
		return fmt.Errorf("frontmatter name is required")
	}
	if !namePattern.MatchString(s.Name) {
		return fmt.Errorf("invalid skill name %q", s.Name)
	}
	if filepath.Base(dir) != s.Name {
		return fmt.Errorf("skill name %q does not match directory %q", s.Name, filepath.Base(dir))
	}
	if s.Description == "" {
		return fmt.Errorf("description is required")
	}
	if len(s.Description) > 1024 {
		return fmt.Errorf("description too long")
	}
	sc := bufio.NewScanner(strings.NewReader(s.Body))
	lines := 0
	for sc.Scan() {
		lines++
	}
	if lines > 500 {
		return fmt.Errorf("skill body exceeds recommended 500 lines")
	}
	return sc.Err()
}

func builtins() []Skill {
	return []Skill{{
		Name:        "skill-creator",
		Description: "当用户要求创建/新增一个技能(skill)时使用。包含技能目录结构、SKILL.md 格式规范与验证流程。",
		Dir:         "(内置)", Builtin: true,
		Body: `# Skill Creator

创建项目级技能时优先写入 .ux-agent/skills/<name>/SKILL.md；需要跨工具共享时可以使用 .claude/skills 或 .agents/skills。

## SKILL.md
---
name: skill-name
description: 当用户……时使用。说明做什么与何时触发。
---

# 指令正文
给出可执行步骤、命令、模板和常见坑。

要求：name 使用小写 kebab-case，必须和目录名一致；description 不超过 1024 字符；长资料放 references/，脚本放 scripts/。创建后重新扫描并用 /skills <name> 验证。`,
	}}
}
