package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/terminal/repl"
	"github.com/codewandler/agentsdk/terminal/ui"
)

type Config struct {
	Resources Resources

	AgentName string
	Task      string

	Workspace          string
	SessionsDir        string
	DefaultSessionsDir string
	Session            string
	ContinueLast       bool

	Inference      agent.InferenceOptions
	ApplyInference bool
	MaxSteps       int
	ApplyMaxSteps  bool
	SystemOverride string
	ToolTimeout    time.Duration
	TotalTimeout   time.Duration
	CacheKeyPrefix string
	Verbose        bool
	Prompt         string

	AgentOptions []agent.Option
	AppOptions   []app.Option

	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func Run(ctx context.Context, cfg Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	in := cfg.In
	if in == nil {
		in = os.Stdin
	}
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := cfg.Err
	if errOut == nil {
		errOut = os.Stderr
	}
	workspace := cfg.Workspace
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		workspace = wd
	}
	if abs, err := filepath.Abs(workspace); err == nil {
		workspace = abs
	}
	sessionsDir, err := resolveSessionsDir(cfg)
	if err != nil {
		return err
	}
	resumePath, err := ResolveSessionPath(sessionsDir, cfg.Session, cfg.ContinueLast)
	if err != nil {
		return err
	}
	if cfg.Resources == nil {
		return fmt.Errorf("cli: resources are required")
	}
	resolved, err := cfg.Resources.Resolve()
	if err != nil {
		return err
	}
	name, err := resolved.ResolveDefaultAgent(cfg.AgentName)
	if err != nil {
		return err
	}
	if err := resolved.UpdateAgentSpec(name, func(spec *agent.Spec) {
		if cfg.ApplyInference {
			spec.Inference = cfg.Inference
		}
		if cfg.ApplyMaxSteps {
			spec.MaxSteps = cfg.MaxSteps
		}
		if strings.TrimSpace(cfg.SystemOverride) != "" {
			spec.System = cfg.SystemOverride
		}
	}); err != nil {
		return err
	}

	appOpts := []app.Option{
		app.WithOutput(out),
		app.WithBundle(resolved.Bundle),
		app.WithDefaultAgent(name),
		app.WithDefaultSkillSourceDiscovery(app.SkillSourceDiscovery{WorkspaceDir: workspace}),
		app.WithAgentWorkspace(workspace),
		app.WithAgentOutput(out),
		app.WithAgentTerminalUI(true),
		app.WithAgentVerbose(cfg.Verbose),
	}
	if cfg.ToolTimeout > 0 {
		appOpts = append(appOpts, app.WithAgentToolTimeout(cfg.ToolTimeout))
	}
	if sessionsDir != "" {
		appOpts = append(appOpts, app.WithAgentSessionStoreDir(sessionsDir))
	}
	if cfg.CacheKeyPrefix != "" {
		appOpts = append(appOpts, app.WithAgentCacheKeyPrefix(cfg.CacheKeyPrefix))
	}
	appOpts = append(appOpts, cfg.AppOptions...)
	application, err := app.New(appOpts...)
	if err != nil {
		return err
	}

	instOpts := append([]agent.Option(nil), cfg.AgentOptions...)
	if resumePath != "" {
		instOpts = append(instOpts, agent.WithResumeSession(resumePath))
	}
	if _, err := application.InstantiateDefaultAgent(instOpts...); err != nil {
		return err
	}

	if strings.TrimSpace(cfg.Task) != "" {
		runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt)
		defer stopSignals()
		cancel := func() {}
		if cfg.TotalTimeout > 0 {
			runCtx, cancel = context.WithTimeout(runCtx, cfg.TotalTimeout)
		}
		defer cancel()
		_, err := application.Send(runCtx, cfg.Task)
		fmt.Fprintln(out)
		ui.PrintSessionUsage(out, application.SessionID(), application.Tracker().Aggregate())
		if errors.Is(err, agent.ErrMaxStepsReached) {
			fmt.Fprintf(errOut, "Warning: %v\n", err)
			return nil
		}
		return err
	}

	prompt := cfg.Prompt
	if prompt == "" {
		prompt = "> "
	}
	return repl.Run(ctx, application, in, repl.WithPrompt(prompt))
}

func resolveSessionsDir(cfg Config) (string, error) {
	dir := cfg.SessionsDir
	if dir == "" {
		dir = cfg.DefaultSessionsDir
	}
	if dir == "" {
		return "", nil
	}
	return filepath.Abs(dir)
}
