package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/llmadapter/unified"
	"github.com/spf13/cobra"
)

type ModelCompleter func(toComplete string) []string

type CommandConfig struct {
	Name  string
	Use   string
	Short string
	Long  string

	Resources   Resources
	ResourceArg bool
	AgentFlag   bool

	DefaultAgent       string
	DefaultSessionsDir string
	CacheKeyPrefix     string
	Prompt             string

	DefaultInference      agent.InferenceOptions
	DefaultMaxSteps       int
	DefaultToolTimeout    time.Duration
	ApplyDefaultInference bool
	ApplyDefaultMaxSteps  bool

	ModelCompleter ModelCompleter

	AgentOptions []agent.Option
	AppOptions   []app.Option

	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func NewCommand(cfg CommandConfig) *cobra.Command {
	inference := cfg.DefaultInference
	if inference == (agent.InferenceOptions{}) {
		inference = agent.DefaultInferenceOptions()
	}
	maxSteps := cfg.DefaultMaxSteps
	if maxSteps <= 0 {
		maxSteps = 30
	}
	toolTimeout := cfg.DefaultToolTimeout
	if toolTimeout <= 0 {
		toolTimeout = 30 * time.Second
	}
	var (
		agentName    = cfg.DefaultAgent
		workspace    string
		systemPrompt string
		totalTimeout time.Duration
		thinkingFlag = string(inference.Thinking)
		effortFlag   = string(inference.Effort)
		session      string
		continueLast bool
		sessionsDir  string
		verbose      bool
	)
	cmd := &cobra.Command{
		Use:           cfg.Use,
		Short:         cfg.Short,
		Long:          cfg.Long,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resources := cfg.Resources
			taskArgs := args
			if cfg.ResourceArg {
				if len(args) == 0 {
					return fmt.Errorf("usage: %s", cmd.UseLine())
				}
				resources = DirResources(args[0])
				taskArgs = args[1:]
			}
			if resources == nil {
				return fmt.Errorf("cli: resources are required")
			}
			if thinkingFlag != "" {
				inference.Thinking = agent.ThinkingMode(thinkingFlag)
			}
			if effortFlag != "" {
				inference.Effort = unified.ReasoningEffort(effortFlag)
			}
			flags := cmd.Flags()
			applyInference := cfg.ApplyDefaultInference ||
				flags.Changed("model") ||
				flags.Changed("max-tokens") ||
				flags.Changed("temperature") ||
				flags.Changed("thinking") ||
				flags.Changed("effort")
			runCfg := Config{
				Resources:          resources,
				AgentName:          agentName,
				Task:               strings.Join(taskArgs, " "),
				Workspace:          workspace,
				SessionsDir:        sessionsDir,
				DefaultSessionsDir: cfg.DefaultSessionsDir,
				Session:            session,
				ContinueLast:       continueLast,
				Inference:          inference,
				ApplyInference:     applyInference,
				MaxSteps:           maxSteps,
				ApplyMaxSteps:      cfg.ApplyDefaultMaxSteps || flags.Changed("max-steps"),
				SystemOverride:     systemPrompt,
				ToolTimeout:        toolTimeout,
				TotalTimeout:       totalTimeout,
				CacheKeyPrefix:     cfg.CacheKeyPrefix,
				Verbose:            verbose,
				Prompt:             cfg.Prompt,
				AgentOptions:       append([]agent.Option(nil), cfg.AgentOptions...),
				AppOptions:         append([]app.Option(nil), cfg.AppOptions...),
				In:                 firstReader(cfg.In, os.Stdin),
				Out:                firstWriter(cfg.Out, cmd.OutOrStdout()),
				Err:                firstWriter(cfg.Err, cmd.ErrOrStderr()),
			}
			return Run(context.Background(), runCfg)
		},
	}
	f := cmd.Flags()
	if cfg.AgentFlag || cfg.ResourceArg {
		f.StringVar(&agentName, "agent", agentName, "Agent name to run")
	}
	f.StringVarP(&inference.Model, "model", "m", inference.Model, "Model alias or full path")
	f.StringVarP(&workspace, "workspace", "w", "", "Working directory (default: $PWD)")
	f.IntVar(&maxSteps, "max-steps", maxSteps, "Maximum agent loop iterations per turn")
	f.IntVar(&inference.MaxTokens, "max-tokens", inference.MaxTokens, "Maximum output tokens per LLM call")
	f.StringVarP(&systemPrompt, "system", "s", "", "Override the system prompt body")
	f.DurationVar(&totalTimeout, "timeout", 0, "Total runtime timeout for one-shot mode (0 = no limit)")
	f.DurationVar(&toolTimeout, "tool-timeout", toolTimeout, "Per-tool call timeout")
	f.Float64Var(&inference.Temperature, "temperature", inference.Temperature, "Sampling temperature 0.0-2.0")
	f.StringVar(&thinkingFlag, "thinking", thinkingFlag, "Thinking mode: auto|on|off")
	f.StringVar(&effortFlag, "effort", effortFlag, "Effort level: low|medium|high")
	f.StringVar(&session, "session", "", "Resume a session by id or JSONL path")
	f.BoolVar(&continueLast, "continue", false, "Resume the most recently active session")
	f.StringVar(&sessionsDir, "sessions-dir", "", "Session storage directory")
	f.BoolVarP(&verbose, "verbose", "v", false, "Show resolved provider/model diagnostics")
	_ = cmd.RegisterFlagCompletionFunc("model", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeModels(cfg.ModelCompleter, toComplete), cobra.ShellCompDirectiveNoFileComp
	})
	cmd.AddCommand(CompletionCommand(cmd, cfg.Name))
	return cmd
}

func CompletionCommand(root *cobra.Command, name string) *cobra.Command {
	if name == "" {
		name = root.Name()
	}
	cmd := &cobra.Command{Use: "completion", Short: "Generate or install shell completion", SilenceUsage: true, SilenceErrors: true}
	cmd.AddCommand(&cobra.Command{Use: "bash", Short: "Generate bash completion script", RunE: func(cmd *cobra.Command, _ []string) error {
		return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
	}})
	cmd.AddCommand(&cobra.Command{Use: "zsh", Short: "Generate zsh completion script", RunE: func(cmd *cobra.Command, _ []string) error {
		return root.GenZshCompletion(cmd.OutOrStdout())
	}})
	cmd.AddCommand(&cobra.Command{Use: "fish", Short: "Generate fish completion script", RunE: func(cmd *cobra.Command, _ []string) error {
		return root.GenFishCompletion(cmd.OutOrStdout(), true)
	}})
	cmd.AddCommand(completionInstallCmd(root, name))
	return cmd
}

func completionInstallCmd(root *cobra.Command, name string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:           "install [bash|zsh|fish]",
		Short:         "Install shell completion to a standard user location",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = strings.ToLower(args[0])
			} else {
				shell = detectShell()
			}
			if shell == "" {
				return fmt.Errorf("unable to detect shell; pass bash, zsh, or fish")
			}
			target, err := completionInstallPath(shell, name, file)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create completion directory: %w", err)
			}
			f, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("create completion file: %w", err)
			}
			defer f.Close()
			switch shell {
			case "bash":
				err = root.GenBashCompletionV2(f, true)
			case "zsh":
				err = root.GenZshCompletion(f)
			case "fish":
				err = root.GenFishCompletion(f, true)
			default:
				return fmt.Errorf("unsupported shell %q", shell)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s completion to %s\n", shell, target)
			fmt.Fprintln(cmd.OutOrStdout(), "Restart your shell or source the file to enable completions.")
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Override installation target file")
	return cmd
}

func DefaultModelCompleter(toComplete string) []string {
	return completeModels(nil, toComplete)
}

func completeModels(completer ModelCompleter, toComplete string) []string {
	if completer != nil {
		return completer(toComplete)
	}
	models := []string{"default", "fast", "powerful", "codex/gpt-5.4"}
	var matches []string
	for _, model := range models {
		if strings.Contains(strings.ToLower(model), strings.ToLower(toComplete)) {
			matches = append(matches, model)
		}
	}
	return matches
}

func detectShell() string {
	shell := strings.ToLower(filepath.Base(os.Getenv("SHELL")))
	switch shell {
	case "bash", "zsh", "fish":
		return shell
	default:
		return ""
	}
}

func completionInstallPath(shell, name, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".local/share/bash-completion/completions", name), nil
	case "zsh":
		return filepath.Join(home, ".zsh/completions", "_"+name), nil
	case "fish":
		return filepath.Join(home, ".config/fish/completions", name+".fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

func firstReader(primary io.Reader, fallback io.Reader) io.Reader {
	if primary != nil {
		return primary
	}
	return fallback
}

func firstWriter(primary io.Writer, fallback io.Writer) io.Writer {
	if primary != nil {
		return primary
	}
	return fallback
}
