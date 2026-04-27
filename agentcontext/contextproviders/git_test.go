package contextproviders

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
)

func TestGitProviderRendersMinimalState(t *testing.T) {
	dir := initGitContextRepo(t)
	provider := Git(WithGitWorkDir(dir), WithGitMode(GitMinimal))

	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Key() != "git" {
		t.Fatalf("key = %q", provider.Key())
	}
	if got, want := len(providerContext.Fragments), 1; got != want {
		t.Fatalf("fragments = %d, want %d", got, want)
	}
	fragment := providerContext.Fragments[0]
	if fragment.Key != "git/state" {
		t.Fatalf("fragment key = %q", fragment.Key)
	}
	for _, want := range []string{
		"root: " + dir,
		"branch:",
		"head:",
		"dirty: false",
	} {
		if !strings.Contains(fragment.Content, want) {
			t.Fatalf("content missing %q: %s", want, fragment.Content)
		}
	}
	if strings.Contains(fragment.Content, "changed_files:") {
		t.Fatalf("minimal mode should not render changed files: %s", fragment.Content)
	}
	if providerContext.Fingerprint == "" {
		t.Fatal("missing provider fingerprint")
	}
	fingerprint, ok, err := provider.StateFingerprint(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || fingerprint != providerContext.Fingerprint {
		t.Fatalf("fingerprint = %q ok=%v, want %q", fingerprint, ok, providerContext.Fingerprint)
	}
}

func TestGitProviderRendersChangedFilesWithCaps(t *testing.T) {
	dir := initGitContextRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := Git(WithGitWorkDir(dir), WithGitMode(GitChangedFiles), WithGitMaxFiles(1))

	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	content := providerContext.Fragments[0].Content
	for _, want := range []string{
		"dirty: true",
		"changed_files:",
		"truncated_files: 1",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q: %s", want, content)
		}
	}
}

func TestGitProviderOffAndNonRepoReturnNoFragments(t *testing.T) {
	for _, provider := range []*GitProvider{
		Git(WithGitMode(GitOff)),
		Git(WithGitWorkDir(t.TempDir()), WithGitMode(GitMinimal)),
	} {
		providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
		if err != nil {
			t.Fatal(err)
		}
		if len(providerContext.Fragments) != 0 {
			t.Fatalf("fragments = %#v, want none", providerContext.Fragments)
		}
	}
}

func TestGitProviderMaxBytesTruncatesContent(t *testing.T) {
	dir := initGitContextRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := Git(WithGitWorkDir(dir), WithGitMode(GitChangedFiles), WithGitMaxBytes(80))

	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	content := providerContext.Fragments[0].Content
	if len(content) > 80 {
		t.Fatalf("content length = %d, want <= 80: %s", len(content), content)
	}
	if !strings.Contains(content, "truncated_bytes: true") {
		t.Fatalf("content missing truncation marker: %s", content)
	}
}

func initGitContextRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitContext(t, dir, "init")
	runGitContext(t, dir, "config", "user.email", "test@example.com")
	runGitContext(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitContext(t, dir, "add", ".")
	runGitContext(t, dir, "commit", "-m", "init")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func runGitContext(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
