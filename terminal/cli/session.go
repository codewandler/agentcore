package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func HomeSessionDir(rel string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	if rel == "" {
		return home, nil
	}
	if filepath.IsAbs(rel) {
		return rel, nil
	}
	return filepath.Join(home, rel), nil
}

func ResolveSessionPath(dir, session string, continueLast bool) (string, error) {
	if session != "" && continueLast {
		return "", fmt.Errorf("--session and --continue cannot be used together")
	}
	if continueLast {
		if dir == "" {
			return "", fmt.Errorf("--continue requires a sessions directory")
		}
		return LatestSessionPath(dir)
	}
	if session == "" {
		return "", nil
	}
	if strings.ContainsAny(session, `/\`) || strings.HasSuffix(session, ".jsonl") {
		path, err := filepath.Abs(session)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("session %s: %w", path, err)
		}
		return path, nil
	}
	if dir == "" {
		return "", fmt.Errorf("--session requires a sessions directory")
	}
	candidates := []string{
		filepath.Join(dir, session+".jsonl"),
		filepath.Join(dir, "*-"+session+".jsonl"),
	}
	var matches []string
	for _, pattern := range candidates {
		found, err := filepath.Glob(pattern)
		if err != nil {
			return "", err
		}
		matches = append(matches, found...)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("session %q not found in %s", session, dir)
	}
	return NewestPath(matches)
}

func LatestSessionPath(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no sessions found in %s", dir)
	}
	return NewestPath(matches)
}

func NewestPath(paths []string) (string, error) {
	var newest string
	var newestMod time.Time
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if newest == "" || info.ModTime().After(newestMod) {
			newest = path
			newestMod = info.ModTime()
		}
	}
	return newest, nil
}
