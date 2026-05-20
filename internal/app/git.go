package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// queryGitStatus executes git commands to determine the current branch name and dirty status.
func queryGitStatus(ctx context.Context, workspacePath string) string {
	if workspacePath == "" {
		return "unknown"
	}

	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	absWorkspace, err := filepath.Abs(workspacePath)
	if err != nil {
		return "unknown"
	}

	// 1. Check if the workspace directory is a git repository without
	// discovering a parent repository through mage's workspace-local TMPDIR.
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = absWorkspace
	cmd.Env = gitExactWorkspaceEnv(absWorkspace)
	if err := cmd.Run(); err != nil {
		return "not a repository"
	}

	// 2. Query branch/HEAD
	cmd = exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = absWorkspace
	cmd.Env = gitExactWorkspaceEnv(absWorkspace)
	branchBytes, err := cmd.Output()
	branch := ""
	if err != nil {
		// Detached HEAD state: try commit SHA
		cmd = exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
		cmd.Dir = absWorkspace
		cmd.Env = gitExactWorkspaceEnv(absWorkspace)
		shaBytes, err2 := cmd.Output()
		if err2 != nil {
			branch = "HEAD"
		} else {
			branch = "HEAD (" + strings.TrimSpace(string(shaBytes)) + ")"
		}
	} else {
		branch = strings.TrimSpace(string(branchBytes))
	}

	// 3. Query porcelain status to check if dirty
	cmd = exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = absWorkspace
	cmd.Env = gitExactWorkspaceEnv(absWorkspace)
	statusBytes, err := cmd.Output()
	if err != nil {
		return branch
	}

	hasChanges := len(strings.TrimSpace(string(statusBytes))) > 0
	if hasChanges {
		return branch + " (dirty)"
	}
	return branch
}

func gitExactWorkspaceEnv(workspacePath string) []string {
	parent := filepath.Dir(workspacePath)
	env := os.Environ()
	return append(env, "GIT_CEILING_DIRECTORIES="+parent)
}
