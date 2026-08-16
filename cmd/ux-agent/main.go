package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/app"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/logging"
	"github.com/ViudiraTech/Uinxed-Agent/internal/tui"
)

var (
	version   = "2.0.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

type options struct {
	key, base, model, provider, theme, session, configDir string
	debug, noMouse, reset, showVersion                    bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ux-agent:", err)
		os.Exit(1)
	}
}

func run() error {
	var o options
	flag.StringVar(&o.key, "key", "", "API key for the active provider")
	flag.StringVar(&o.base, "base", "", "base URL for the active provider")
	flag.StringVar(&o.model, "model", "", "model id")
	flag.StringVar(&o.provider, "provider", "", "provider id")
	flag.StringVar(&o.theme, "theme", "", "theme: uinxed, dark, light")
	flag.StringVar(&o.session, "session", "", "resume session by id or exact name")
	flag.StringVar(&o.configDir, "config-dir", "", "override config directory")
	flag.BoolVar(&o.noMouse, "no-mouse", false, "disable terminal mouse capture")
	flag.BoolVar(&o.debug, "debug", false, "enable debug logging")
	flag.BoolVar(&o.reset, "reset", false, "remove all ux-agent config and sessions")
	flag.BoolVar(&o.showVersion, "version", false, "print version")
	shortV := flag.Bool("v", false, "print version")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Uinxed Agent %s

Usage:
  ux-agent [options]

Options:
`, version)
		flag.PrintDefaults()
	}
	flag.Parse()
	if o.showVersion || *shortV {
		fmt.Printf("ux-agent %s (commit %s, built %s)\n", version, commit, buildDate)
		return nil
	}

	cfg, err := config.NewStore(o.configDir)
	if err != nil {
		return err
	}
	if o.reset {
		if err := cfg.Reset(); err != nil {
			return err
		}
		fmt.Println("配置与会话已清除")
		return nil
	}
	if err := applyCLI(cfg, o); err != nil {
		return err
	}
	snap := cfg.Snapshot()
	stateDir := defaultStateDir()
	log, closer, err := logging.Open(stateDir, snap.Debug || o.debug)
	if err != nil {
		return err
	}
	defer closer.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctrl, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer ctrl.Close()
	sess, err := resolveSession(ctx, ctrl, o.session)
	if err != nil {
		return err
	}
	model := tui.New(ctx, ctrl, sess)
	p := tea.NewProgram(model)
	_, err = p.Run()
	return err
}

func applyCLI(s *config.Store, o options) error {
	if o.provider != "" {
		if err := s.SetActiveProvider(o.provider); err != nil {
			return err
		}
	}
	if o.key != "" {
		if err := s.SetProviderKey(s.Snapshot().ActiveProvider, o.key); err != nil {
			return err
		}
	}
	return s.Update(func(c *config.Config) error {
		if o.base != "" {
			base := strings.TrimRight(strings.TrimSpace(o.base), "/")
			c.BaseURL = base
			for i := range c.Providers {
				if c.Providers[i].ID == c.ActiveProvider {
					c.Providers[i].BaseURL = base
				}
			}
		}
		if o.model != "" {
			c.Model = o.model
		}
		if o.theme != "" {
			v := strings.ToLower(o.theme)
			if v != "uinxed" && v != "dark" && v != "light" {
				return fmt.Errorf("unknown theme %q", o.theme)
			}
			c.Theme = v
		}
		if o.noMouse {
			c.Mouse = false
		}
		if o.debug {
			c.Debug = true
		}
		return nil
	})
}

func resolveSession(ctx context.Context, c *app.Controller, want string) (domain.Session, error) {
	if strings.TrimSpace(want) == "" {
		return c.EnsureSession(ctx)
	}
	if s, err := c.LoadSession(ctx, want); err == nil {
		_ = c.SwitchSession(ctx, s.ID)
		return s, nil
	}
	all, err := c.ListSessions(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	for _, s := range all {
		if s.Name == want {
			_ = c.SwitchSession(ctx, s.ID)
			return s, nil
		}
	}
	return domain.Session{}, fmt.Errorf("session %q not found", want)
}

func defaultStateDir() string {
	if runtime.GOOS == "windows" {
		if d, e := os.UserConfigDir(); e == nil {
			return filepath.Join(d, "ux-agent", "state")
		}
	}
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "ux-agent")
	}
	if h, e := os.UserHomeDir(); e == nil {
		return filepath.Join(h, ".local", "state", "ux-agent")
	}
	return filepath.Join(os.TempDir(), "ux-agent-state")
}
