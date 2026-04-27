package contextproviders

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
)

type GitMode string

const (
	GitOff          GitMode = "off"
	GitMinimal      GitMode = "minimal"
	GitChangedFiles GitMode = "changed_files"
)

const (
	defaultGitMaxFiles = 50
	defaultGitMaxBytes = 4000
	defaultGitTimeout  = 5 * time.Second
)

type GitOption func(*GitProvider)

type GitProvider struct {
	key      agentcontext.ProviderKey
	workDir  string
	mode     GitMode
	maxFiles int
	maxBytes int
	timeout  time.Duration
	runGit   func(context.Context, string, ...string) (string, error)
}

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

func (p *GitProvider) Key() agentcontext.ProviderKey {
	if p == nil || p.key == "" {
		return "git"
	}
	return p.key
}

func (p *GitProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	content, err := p.content(ctx)
	if err != nil {
		return agentcontext.ProviderContext{}, err
	}
	if content == "" {
		return agentcontext.ProviderContext{Fingerprint: contentFingerprint("git", content)}, nil
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
		Fingerprint: contentFingerprint("git", content),
	}, nil
}

func (p *GitProvider) StateFingerprint(ctx context.Context, _ agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	content, err := p.content(ctx)
	if err != nil {
		return "", false, err
	}
	return contentFingerprint("git", content), true, nil
}

func (p *GitProvider) content(ctx context.Context) (string, error) {
	if p == nil || p.mode == GitOff {
		return "", nil
	}
	workDir := p.resolvedWorkDir()
	inside, err := p.git(ctx, workDir, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return "", nil
	}
	root := trimGitOutput(p.git(ctx, workDir, "rev-parse", "--show-toplevel"))
	branch := trimGitOutput(p.git(ctx, workDir, "rev-parse", "--abbrev-ref", "HEAD"))
	head := trimGitOutput(p.git(ctx, workDir, "rev-parse", "--short", "HEAD"))
	status, err := p.git(ctx, workDir, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return "", err
	}
	changes := parseGitStatus(status)
	var b strings.Builder
	writeLine(&b, "root", root)
	writeLine(&b, "branch", branch)
	writeLine(&b, "head", head)
	writeLine(&b, "dirty", strconv.FormatBool(len(changes) > 0))
	if p.mode == GitChangedFiles && len(changes) > 0 {
		writeGitChanges(&b, changes, p.maxFilesOrDefault())
	}
	return limitGitContent(b.String(), p.maxBytesOrDefault()), nil
}

func (p *GitProvider) resolvedWorkDir() string {
	if p != nil && p.workDir != "" {
		if abs, err := filepath.Abs(p.workDir); err == nil {
			return abs
		}
		return p.workDir
	}
	wd, err := filepath.Abs(".")
	if err != nil {
		return "."
	}
	return wd
}

func (p *GitProvider) git(ctx context.Context, workDir string, args ...string) (string, error) {
	if p != nil && p.runGit != nil {
		return p.runGit(ctx, workDir, args...)
	}
	timeout := defaultGitTimeout
	if p != nil && p.timeout > 0 {
		timeout = p.timeout
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return "", cmdCtx.Err()
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
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

func trimGitOutput(out string, err error) string {
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func parseGitStatus(status string) []string {
	lines := strings.Split(status, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func writeGitChanges(b *strings.Builder, changes []string, maxFiles int) {
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
		b.WriteString(change)
	}
	if limit < len(changes) {
		b.WriteString("\ntruncated_files: ")
		b.WriteString(strconv.Itoa(len(changes) - limit))
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
