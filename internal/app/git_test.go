package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueryGitStatusNonRepository(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	status := queryGitStatus(context.Background(), tmp)
	if status != "not a repository" {
		t.Fatalf("expected 'not a repository', got %q", status)
	}
}

func TestQueryGitStatusRepository(t *testing.T) {
	// Not running parallel since it executes external git commands on temp dir
	tmp := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmp
	if err := cmd.Run(); err != nil {
		t.Skip("git not available in test environment, skipping repo status test")
		return
	}

	// Configure local user info to allow committing
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmp
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmp
	_ = cmd.Run()

	// Initial query - might be master/main or empty
	status := queryGitStatus(context.Background(), tmp)
	if status == "not a repository" || status == "unknown" {
		t.Fatalf("unexpected git status on initialized repo: %q", status)
	}

	// Create a file and verify dirty status
	filePath := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	status = queryGitStatus(context.Background(), tmp)
	if !strings.Contains(status, "dirty") {
		t.Fatalf("expected status to be dirty after file creation, got %q", status)
	}

	// Commit file to make clean
	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = tmp
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "commit", "--no-gpg-sign", "-m", "initial commit")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v: %s", err, stderr.String())
	}

	status = queryGitStatus(context.Background(), tmp)
	if strings.Contains(status, "dirty") {
		t.Fatalf("expected status to be clean after commit, got %q", status)
	}
}
