package agentdir

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/resource"
)

func sourceForCandidate(path string, base string, scope resource.Scope) resource.SourceRef {
	ecosystem := "agents"
	if strings.Contains(filepath.ToSlash(path), ".claude") {
		ecosystem = "claude"
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = path
	}
	source := resource.SourceRef{
		Ecosystem: ecosystem,
		Scope:     scope,
		Root:      path,
		Path:      rel,
		Trust:     resource.TrustDeclarative,
	}
	source.ID = resource.QualifiedID(source, "source", "", rel)
	return source
}

func materializeSource(base string, raw string, policy resource.DiscoveryPolicy) (string, resource.SourceRef, error) {
	if raw == "" {
		return "", resource.SourceRef{}, fmt.Errorf("agentdir: source is required")
	}
	if strings.HasPrefix(raw, "git+") {
		if !policy.AllowRemote {
			return "", resource.SourceRef{}, fmt.Errorf("agentdir: remote source %q requires allow_remote", raw)
		}
		return materializeGitSource(base, raw, policy)
	}
	if strings.HasPrefix(raw, "file://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", resource.SourceRef{}, fmt.Errorf("agentdir: parse source %q: %w", raw, err)
		}
		p := u.Path
		return p, sourceForCandidate(p, base, resource.ScopeProject), nil
	}
	p := raw
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return p, sourceForCandidate(p, base, resource.ScopeProject), nil
}

func isRemoteSource(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), "git+")
}

func materializeGitSource(base string, raw string, policy resource.DiscoveryPolicy) (string, resource.SourceRef, error) {
	cloneURL := strings.TrimPrefix(raw, "git+")
	u, err := url.Parse(cloneURL)
	if err != nil {
		return "", resource.SourceRef{}, fmt.Errorf("agentdir: parse git source %q: %w", raw, err)
	}
	ref := u.Fragment
	if ref == "" {
		ref = "HEAD"
	}
	u.Fragment = ""
	cloneURL = u.String()
	cacheRoot := policy.TrustStoreDir
	if cacheRoot == "" {
		cacheRoot = filepath.Join(base, ".agentsdk")
	}
	repoPath := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
	if repoPath == "" {
		repoPath = "repo"
	}
	worktree := filepath.Join(cacheRoot, "cache", "git", u.Host, filepath.FromSlash(repoPath), "refs", sanitizeRef(ref), "worktree")
	if err := syncGitWorktree(worktree, cloneURL, ref); err != nil {
		return "", resource.SourceRef{}, fmt.Errorf("agentdir: materialize git source %s: %w", raw, err)
	}
	if err := writeGitSourceMeta(worktree, raw, cloneURL, ref); err != nil {
		return "", resource.SourceRef{}, err
	}
	source := resource.SourceRef{
		Ecosystem: "agents",
		Scope:     resource.ScopeGit,
		Root:      worktree,
		Path:      raw,
		Ref:       ref,
		Trust:     resource.TrustDeclarative,
	}
	source.ID = resource.QualifiedID(source, "source", "", raw)
	return worktree, source, nil
}

func syncGitWorktree(worktree string, cloneURL string, ref string) error {
	if _, err := os.Stat(filepath.Join(worktree, ".git")); err == nil {
		return refreshGitWorktree(worktree, ref)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		return err
	}
	args := []string{"clone", "--depth", "1"}
	if ref != "HEAD" {
		args = append(args, "--branch", ref)
	}
	args = append(args, cloneURL, worktree)
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		_ = os.RemoveAll(worktree)
		return fmt.Errorf("git clone: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func refreshGitWorktree(worktree string, ref string) error {
	if ref == "HEAD" {
		if out, err := runGit(worktree, "fetch", "--depth", "1", "origin"); err != nil {
			return fmt.Errorf("git fetch: %w: %s", err, out)
		}
		if out, err := runGit(worktree, "reset", "--hard", "FETCH_HEAD"); err != nil {
			return fmt.Errorf("git reset: %w: %s", err, out)
		}
		return nil
	}
	if out, err := runGit(worktree, "fetch", "--depth", "1", "origin", ref); err != nil {
		return fmt.Errorf("git fetch %s: %w: %s", ref, err, out)
	}
	if out, err := runGit(worktree, "checkout", "--detach", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("git checkout %s: %w: %s", ref, err, out)
	}
	return nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func writeGitSourceMeta(worktree string, raw string, cloneURL string, ref string) error {
	commit := ""
	if out, err := runGit(worktree, "rev-parse", "HEAD"); err == nil {
		commit = out
	}
	meta := map[string]string{
		"source":          raw,
		"clone_url":       cloneURL,
		"ref":             ref,
		"resolved_commit": commit,
		"fetched_at":      time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(filepath.Dir(worktree), "meta.json"), append(data, '\n'), 0o644)
}

func sanitizeRef(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.ReplaceAll(ref, "/", "_")
	ref = strings.ReplaceAll(ref, "\\", "_")
	ref = strings.ReplaceAll(ref, ":", "_")
	if ref == "" {
		return "HEAD"
	}
	return ref
}
