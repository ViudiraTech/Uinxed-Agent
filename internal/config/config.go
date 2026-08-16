package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL = "http://localhost:8080/v1"
	DefaultModel   = "glm-4-flash"
)

type Provider struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	BaseURL          string            `json:"baseUrl"`
	APIKey           string            `json:"apiKey,omitempty"`
	APIKeyEnc        string            `json:"apiKeyEnc,omitempty"`
	Models           []string          `json:"models"`
	DefaultModel     string            `json:"defaultModel"`
	Builtin          bool              `json:"builtin"`
	SupportsThinking bool              `json:"supportsThinking,omitempty"`
	SupportsEffort   bool              `json:"supportsEffort,omitempty"`
	ReasoningEffort  string            `json:"reasoningEffort,omitempty"`
	WireAPI          string            `json:"wireApi,omitempty"`
	RequiresAuth     *bool             `json:"requiresAuth,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
}

type Config struct {
	Version                int        `json:"version"`
	BaseURL                string     `json:"baseUrl,omitempty"`
	Model                  string     `json:"model"`
	Storage                string     `json:"storage"`
	Providers              []Provider `json:"providers"`
	ActiveProvider         string     `json:"activeProvider"`
	ActiveSessionID        string     `json:"activeSessionId,omitempty"`
	Thinking               bool       `json:"thinking"`
	Effort                 string     `json:"effort"`
	CWD                    string     `json:"cwd,omitempty"`
	Theme                  string     `json:"theme"`
	Mouse                  bool       `json:"mouse"`
	ScrollSpeed            int        `json:"scroll_speed"`
	Sidebar                string     `json:"sidebar"`
	Animations             bool       `json:"animations"`
	StreamRenderIntervalMS int        `json:"stream_render_interval_ms"`
	Debug                  bool       `json:"debug,omitempty"`
}

func BuiltinProviders() []Provider {
	return []Provider{
		{
			ID: "ux-gateway", Name: "本地网关", BaseURL: DefaultBaseURL,
			Models:       []string{"glm-4-flash", "glm-4-flash-proxy"},
			DefaultModel: "glm-4-flash", Builtin: true,
		},
		{
			ID: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1",
			Models:       []string{"deepseek-v4-pro", "deepseek-v4-flash"},
			DefaultModel: "deepseek-v4-flash", Builtin: true, SupportsThinking: true,
		},
		{
			ID: "router", Name: "Router", BaseURL: "https://api.hcnsec.cn/v1",
			Models:       []string{"DeepSeek-V4-Pro", "step-3.7-flash", "step-3.5-flash-2603"},
			DefaultModel: "step-3.7-flash", Builtin: true,
			SupportsThinking: true, SupportsEffort: true,
		},
	}
}

func Defaults() Config {
	return Config{
		Version: 2, BaseURL: DefaultBaseURL, Model: DefaultModel, Storage: "db",
		Providers: BuiltinProviders(), ActiveProvider: "ux-gateway",
		Thinking: true, Effort: "high", Theme: "uinxed", Mouse: true,
		ScrollSpeed: 3, Sidebar: "auto", Animations: true, StreamRenderIntervalMS: 16,
	}
}

type Store struct {
	dir  string
	file string
	mu   sync.RWMutex
	cfg  Config
}

func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ux-agent"), nil
}

func NewStore(dir string) (*Store, error) {
	if dir == "" {
		var err error
		dir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}
	s := &Store{dir: dir, file: filepath.Join(dir, "config.json")}
	cfg, err := s.read()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Preserve a malformed config for recovery; boot with defaults.
		_ = backupFile(s.file, ".invalid")
	}
	s.cfg = mergeDefaults(cfg)
	return s, nil
}

func (s *Store) Dir() string  { return s.dir }
func (s *Store) File() string { return s.file }

func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.cfg)
}

func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.read()
	if err != nil {
		return err
	}
	s.cfg = mergeDefaults(cfg)
	return nil
}

func (s *Store) Update(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneConfig(s.cfg)
	if err := fn(&next); err != nil {
		return err
	}
	next = mergeDefaults(next)
	if err := validate(&next); err != nil {
		return err
	}
	if err := s.write(next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

func (s *Store) SetActiveProvider(id string) error {
	return s.Update(func(c *Config) error {
		p := providerByID(c.Providers, id)
		if p == nil {
			return fmt.Errorf("unknown provider %q", id)
		}
		c.ActiveProvider = p.ID
		c.BaseURL = p.BaseURL
		if p.DefaultModel != "" {
			c.Model = p.DefaultModel
		}
		return nil
	})
}

func (s *Store) UpsertProvider(p Provider, key string) error {
	return s.Update(func(c *Config) error {
		p.ID = strings.TrimSpace(p.ID)
		if p.ID == "" {
			p.ID = slug(p.Name)
		}
		p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		if p.Name == "" || p.BaseURL == "" || p.ID == "" {
			return errors.New("provider name, id and base URL are required")
		}
		if len(p.Models) == 0 {
			p.Models = []string{"default"}
		}
		if p.DefaultModel == "" {
			p.DefaultModel = p.Models[0]
		}
		p.APIKey = ""
		if key != "" {
			enc, err := EncryptSecret(key)
			if err != nil {
				return err
			}
			p.APIKeyEnc = enc
		}
		for i := range c.Providers {
			if c.Providers[i].ID == p.ID {
				if p.APIKeyEnc == "" {
					p.APIKeyEnc = c.Providers[i].APIKeyEnc
				}
				c.Providers[i] = p
				return nil
			}
		}
		c.Providers = append(c.Providers, p)
		return nil
	})
}

func (s *Store) RemoveProvider(id string) error {
	return s.Update(func(c *Config) error {
		if id == c.ActiveProvider {
			return errors.New("不能删除当前活动的提供商")
		}
		out := c.Providers[:0]
		for _, p := range c.Providers {
			if p.ID != id {
				out = append(out, p)
			}
		}
		c.Providers = out
		return nil
	})
}

func (s *Store) SetProviderKey(id, key string) error {
	return s.Update(func(c *Config) error {
		for i := range c.Providers {
			if c.Providers[i].ID != id {
				continue
			}
			c.Providers[i].APIKey = ""
			if key == "" {
				c.Providers[i].APIKeyEnc = ""
				return nil
			}
			enc, err := EncryptSecret(key)
			if err != nil {
				return err
			}
			c.Providers[i].APIKeyEnc = enc
			return nil
		}
		return fmt.Errorf("unknown provider %q", id)
	})
}

func (s *Store) ProviderKey(id string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := providerByID(s.cfg.Providers, id)
	if p == nil {
		return "", fmt.Errorf("unknown provider %q", id)
	}
	if p.APIKeyEnc != "" {
		v, err := DecryptSecret(p.APIKeyEnc)
		if err == nil && v != "" {
			return v, nil
		}
		if p.APIKey == "" {
			return "", err
		}
	}
	return p.APIKey, nil
}

func (s *Store) ActiveProvider() Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p := providerByID(s.cfg.Providers, s.cfg.ActiveProvider); p != nil {
		return cloneProvider(*p)
	}
	return cloneProvider(s.cfg.Providers[0])
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.RemoveAll(s.dir); err != nil {
		return err
	}
	s.cfg = Defaults()
	return nil
}

func (s *Store) read() (Config, error) {
	raw, err := os.ReadFile(s.file)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (s *Store) write(cfg Config) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	var raw []byte
	if cfg.Storage == "config" {
		base := map[string]any{}
		if old, err := os.ReadFile(s.file); err == nil {
			_ = json.Unmarshal(old, &base)
		}
		known, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		var km map[string]any
		if err := json.Unmarshal(known, &km); err != nil {
			return err
		}
		for k, v := range km {
			base[k] = v
		}
		raw, err = json.MarshalIndent(base, "", "  ")
		if err != nil {
			return err
		}
	} else {
		var err error
		raw, err = json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(s.dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(s.file); err == nil {
		_ = backupFile(s.file, ".bak")
	}
	return os.Rename(name, s.file)
}

func mergeDefaults(in Config) Config {
	d := Defaults()
	if in.Version != 0 {
		d.Version = in.Version
	}
	if in.BaseURL != "" {
		d.BaseURL = in.BaseURL
	}
	if in.Model != "" {
		d.Model = in.Model
	}
	if in.Storage != "" {
		d.Storage = in.Storage
	}
	if len(in.Providers) > 0 {
		d.Providers = ensureProviders(in.Providers)
	}
	if in.ActiveProvider != "" {
		d.ActiveProvider = in.ActiveProvider
	}
	if in.ActiveSessionID != "" {
		d.ActiveSessionID = in.ActiveSessionID
	}
	// JSON booleans cannot distinguish omitted from false. Legacy config had thinking default true.
	// Preserve explicit false when config exists by looking at the populated config shape.
	if in.Version != 0 || len(in.Providers) > 0 || in.Model != "" {
		d.Thinking = in.Thinking
		d.Mouse = in.Mouse
		d.Animations = in.Animations
	}
	if in.Effort != "" {
		d.Effort = in.Effort
	}
	if in.CWD != "" {
		d.CWD = in.CWD
	}
	if in.Theme != "" {
		d.Theme = in.Theme
	}
	if in.ScrollSpeed != 0 {
		d.ScrollSpeed = in.ScrollSpeed
	}
	if in.Sidebar != "" {
		d.Sidebar = in.Sidebar
	}
	if in.StreamRenderIntervalMS != 0 {
		d.StreamRenderIntervalMS = in.StreamRenderIntervalMS
	}
	d.Debug = in.Debug
	if in.Version == 0 {
		d.Version = 2
		// Legacy files lacked these UI fields: maintain historical default-on behavior.
		d.Mouse, d.Animations = true, true
	}
	_ = validate(&d)
	return d
}

func ensureProviders(existing []Provider) []Provider {
	out := make([]Provider, 0, len(existing)+3)
	for _, p := range existing {
		if tpl := providerByID(BuiltinProviders(), p.ID); tpl != nil {
			p = mergeProvider(*tpl, p)
		}
		out = append(out, p)
	}
	for _, p := range BuiltinProviders() {
		if providerByID(out, p.ID) == nil {
			out = append(out, p)
		}
	}
	return out
}

func mergeProvider(base, override Provider) Provider {
	out := base
	if override.Name != "" {
		out.Name = override.Name
	}
	if override.BaseURL != "" {
		out.BaseURL = override.BaseURL
	}
	if override.APIKey != "" {
		out.APIKey = override.APIKey
	}
	if override.APIKeyEnc != "" {
		out.APIKeyEnc = override.APIKeyEnc
	}
	if len(override.Models) > 0 {
		out.Models = append([]string(nil), override.Models...)
	}
	if override.DefaultModel != "" {
		out.DefaultModel = override.DefaultModel
	}
	out.Builtin = override.Builtin || base.Builtin
	out.SupportsThinking = override.SupportsThinking || base.SupportsThinking
	out.SupportsEffort = override.SupportsEffort || base.SupportsEffort
	if override.ReasoningEffort != "" {
		out.ReasoningEffort = override.ReasoningEffort
	}
	if override.WireAPI != "" {
		out.WireAPI = override.WireAPI
	}
	if override.RequiresAuth != nil {
		out.RequiresAuth = override.RequiresAuth
	}
	if override.Headers != nil {
		out.Headers = cloneMap(override.Headers)
	}
	return out
}

func validate(c *Config) error {
	if c.Storage != "db" && c.Storage != "config" {
		c.Storage = "db"
	}
	if c.ScrollSpeed < 1 {
		c.ScrollSpeed = 1
	}
	if c.ScrollSpeed > 20 {
		c.ScrollSpeed = 20
	}
	if c.StreamRenderIntervalMS < 8 {
		c.StreamRenderIntervalMS = 8
	}
	if c.StreamRenderIntervalMS > 100 {
		c.StreamRenderIntervalMS = 100
	}
	switch c.Effort {
	case "low", "medium", "high", "xhigh", "max", "supercode":
	default:
		c.Effort = "high"
	}
	switch c.Theme {
	case "uinxed", "dark", "light":
	default:
		c.Theme = "uinxed"
	}
	if len(c.Providers) == 0 {
		return errors.New("at least one provider is required")
	}
	if providerByID(c.Providers, c.ActiveProvider) == nil {
		c.ActiveProvider = c.Providers[0].ID
	}
	return nil
}

func providerByID(ps []Provider, id string) *Provider {
	for i := range ps {
		if ps[i].ID == id {
			return &ps[i]
		}
	}
	return nil
}

func cloneConfig(c Config) Config {
	out := c
	out.Providers = make([]Provider, len(c.Providers))
	for i, p := range c.Providers {
		out.Providers[i] = cloneProvider(p)
	}
	return out
}

func cloneProvider(p Provider) Provider {
	p.Models = append([]string(nil), p.Models...)
	p.Headers = cloneMap(p.Headers)
	return p
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func backupFile(path, suffix string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dst := path + suffix + "." + time.Now().UTC().Format("20060102T150405Z")
	return os.WriteFile(dst, raw, 0o600)
}

func RuntimeSummary() string {
	return fmt.Sprintf("%s/%s go=%s", runtime.GOOS, runtime.GOARCH, runtime.Version())
}
