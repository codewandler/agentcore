package contextproviders

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
)

// GitMode controls how much git state is included in context.
type GitMode string

const (
	GitOff          GitMode = "off"
	GitMinimal      GitMode = "minimal"
	GitChangedFiles GitMode = "changed_files"
	GitSummary      GitMode = "summary"
)

const (
	defaultGitMaxFiles = 50
	defaultGitMaxBytes = 4000
	defaultGitTimeout  = 5 * time.Second
)

// GitOption configures a GitProvider.
type GitOption func(*GitProvider)

// GitProvider renders git repository state as context. It delegates the
// baseline key/value lines (root, branch, head) to a [CmdProvider] and adds
// dirty state and optional changed-file lists on top.
type GitProvider struct {
	key      agentcontext.ProviderKey
	workDir  string
	mode     GitMode
	maxFiles int
	maxBytes int
	timeout  time.Duration
	runCmd   func(ctx context.Context, workDir string, name string, args ...string) (string, error)
}

// Git creates a git context provider.
func Git(opts ...GitOption) *GitProvider {
	p := &GitProvider{
		key:      "git",
		mode:     GitMinimal,
		maxFiles: defaultGitMaxFiles,
		maxBytes: defaultGitMaxBytes,
		timeout:  defaultGitTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

func WithGitKey(key agentcontext.ProviderKey) GitOption {
	return func(p *GitProvider) { p.key = key }
}

func WithGitWorkDir(workDir string) GitOption {
	return func(p *GitProvider) { p.workDir = workDir }
}

func WithGitMode(mode GitMode) GitOption {
	return func(p *GitProvider) { p.mode = mode }
}

func WithGitMaxFiles(max int) GitOption {
	return func(p *GitProvider) { p.maxFiles = max }
}

func WithGitMaxBytes(max int) GitOption {
	return func(p *GitProvider) { p.maxBytes = max }
}

func WithGitTimeout(timeout time.Duration) GitOption {
	return func(p *GitProvider) { p.timeout = timeout }
}

// WithGitRunner overrides the command runner for testing.
func WithGitRunner(run func(ctx context.Context, workDir string, name string, args ...string) (string, error)) GitOption {
	return func(p *GitProvider) { p.runCmd = run }
}

func (p *GitProvider) Key() agentcontext.ProviderKey {
	if p == nil || p.key == "" {
		return "git"
	}
	return p.key
}

func (p *GitProvider) GetContext(ctx context.Context, req agentcontext.Request) (agentcontext.ProviderContext, error) {
	if p == nil || p.mode == GitOff {
		return agentcontext.ProviderContext{}, nil
	}
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	content, err := p.content(ctx, req)
	if err != nil {
		return agentcontext.ProviderContext{}, err
	}
	fp := contentFingerprint("git", content)
	if content == "" {
		return agentcontext.ProviderContext{Fingerprint: fp}, nil
	}
	return agentcontext.ProviderContext{
		Fragments: []agentcontext.ContextFragment{{
			Key:       "git/state",
			Role:      unified.RoleUser,
			Content:   content,
			Authority: agentcontext.AuthorityUser,
			CachePolicy: agentcontext.CachePolicy{
				Scope: agentcontext.CacheTurn,
			},
		}},
		Fingerprint: fp,
	}, nil
}

func (p *GitProvider) StateFingerprint(ctx context.Context, req agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	content, err := p.content(ctx, req)
	if err != nil {
		return "", false, err
	}
	return contentFingerprint("git", content), true, nil
}

func (p *GitProvider) content(ctx context.Context, req agentcontext.Request) (string, error) {
	// Check if we're inside a git repo.
	inside, err := p.run(ctx, Cmd{Command: "git", Args: []string{"rev-parse", "--is-inside-work-tree"}, Optional: true})
	if err != nil || strings.TrimSpace(inside) != "true" {
		return "", nil
	}

	// Use CmdProvider for the baseline key/value lines (root, branch, head).
	base := p.baseCmdProvider()
	baseResult, err := base.GetContext(ctx, req)
	if err != nil {
		return "", err
	}
	var content string
	if len(baseResult.Fragments) > 0 {
		content = baseResult.Fragments[0].Content
	}

	// Always add dirty state from git status.
	status, err := p.run(ctx, Cmd{Command: "git", Args: []string{"status", "--porcelain=v1", "--untracked-files=normal"}})
	if err != nil {
		// Degrade gracefully: return base content without dirty/changes.
		return content, nil
	}
	changes := parseGitStatus(status)
	var b strings.Builder
	b.WriteString(content)
	writeLine(&b, "dirty", strconv.FormatBool(len(changes) > 0))

	if p.mode == GitSummary {
		writeGitSummary(&b, changes)
	}
	if p.mode == GitChangedFiles && len(changes) > 0 {
		writeGitChanges(&b, changes, p.maxFilesOrDefault())
	}
	return limitGitContent(b.String(), p.maxBytesOrDefault()), nil
}

// baseCmdProvider returns a CmdProvider for the basic git identity lines.
func (p *GitProvider) baseCmdProvider() *CmdProvider {
	opts := []CmdProviderOption{
		WithCmdWorkDir(p.workDir),
		WithCmdTimeout(p.timeout),
	}
	if p.runCmd != nil {
		opts = append(opts, WithCmdRunner(p.runCmd))
	}
	return CmdContext(p.Key(), "git/state", GitMinimalCmds(), opts...)
}

func (p *GitProvider) run(ctx context.Context, cmd Cmd) (string, error) {
	if p.runCmd != nil {
		return p.runCmd(ctx, p.workDir, cmd.Command, cmd.Args...)
	}
	// Delegate to a temporary CmdProvider for consistent timeout/workdir handling.
	tmp := CmdContext("", "", nil, WithCmdWorkDir(p.workDir), WithCmdTimeout(p.timeout))
	return tmp.run(ctx, cmd)
}

func (p *GitProvider) maxFilesOrDefault() int {
	if p == nil || p.maxFiles <= 0 {
		return defaultGitMaxFiles
	}
	return p.maxFiles
}

func (p *GitProvider) maxBytesOrDefault() int {
	if p == nil || p.maxBytes <= 0 {
		return defaultGitMaxBytes
	}
	return p.maxBytes
}

// GitMinimalCmds returns the Cmd list for a minimal git context provider.
// Callers can use this with [CmdContext] to build a standalone CmdProvider
// for git identity without the GitProvider's changed_files/truncation logic.
func GitMinimalCmds() []Cmd {
	return []Cmd{
		{Key: "root", Command: "git", Args: []string{"rev-parse", "--show-toplevel"}, Optional: true},
		{Key: "branch", Command: "git", Args: []string{"rev-parse", "--abbrev-ref", "HEAD"}, Optional: true},
		{Key: "head", Command: "git", Args: []string{"rev-parse", "--short", "HEAD"}, Optional: true},
	}
}

type gitStatusEntry struct {
	raw            string
	indexStatus    byte
	worktreeStatus byte
	path           string
}

func parseGitStatus(status string) []gitStatusEntry {
	lines := strings.Split(status, "\n")
	out := make([]gitStatusEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, parseGitStatusLine(line))
	}
	return out
}

func parseGitStatusLine(line string) gitStatusEntry {
	entry := gitStatusEntry{raw: line}
	if len(line) >= 2 {
		entry.indexStatus = line[0]
		entry.worktreeStatus = line[1]
	}
	if len(line) > 3 {
		entry.path = strings.TrimSpace(line[3:])
	}
	return entry
}

func writeGitSummary(b *strings.Builder, changes []gitStatusEntry) {
	var staged, unstaged, untracked int
	for _, change := range changes {
		switch {
		case change.indexStatus == '?' && change.worktreeStatus == '?':
			untracked++
		default:
			if change.indexStatus != 0 && change.indexStatus != ' ' && change.indexStatus != '?' {
				staged++
			}
			if change.worktreeStatus != 0 && change.worktreeStatus != ' ' && change.worktreeStatus != '?' {
				unstaged++
			}
		}
	}
	writeLine(b, "changed_file_count", strconv.Itoa(len(changes)))
	writeLine(b, "staged", strconv.Itoa(staged))
	writeLine(b, "unstaged", strconv.Itoa(unstaged))
	writeLine(b, "untracked", strconv.Itoa(untracked))
}

func writeGitChanges(b *strings.Builder, changes []gitStatusEntry, maxFiles int) {
	if len(changes) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString("changed_files:")
	limit := len(changes)
	if maxFiles > 0 && limit > maxFiles {
		limit = maxFiles
	}
	for _, change := range changes[:limit] {
		b.WriteString("\n  ")
		b.WriteString(change.raw)
	}
	if limit < len(changes) {
		fmt.Fprintf(b, "\ntruncated_files: %d", len(changes)-limit)
	}
}

func limitGitContent(content string, maxBytes int) string {
	if maxBytes <= 0 || len(content) <= maxBytes {
		return content
	}
	if maxBytes <= len("\ntruncated_bytes: true") {
		return content[:maxBytes]
	}
	suffix := "\ntruncated_bytes: true"
	return strings.TrimRight(content[:maxBytes-len(suffix)], "\n") + suffix
}
