package app

import (
	"context"
	"os/exec"
	"strings"
)

// queryGitStatus executes git commands to determine the current branch name and dirty status.
func queryGitStatus(ctx context.Context, workspacePath string) string {
	if workspacePath == "" {
		return "unknown"
	}

	// 1. Check if the directory is a git repository
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = workspacePath
	if err := cmd.Run(); err != nil {
		return "not a repository"
	}

	// 2. Query branch/HEAD
	cmd = exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = workspacePath
	branchBytes, err := cmd.Output()
	branch := ""
	if err != nil {
		// Detached HEAD state: try commit SHA
		cmd = exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
		cmd.Dir = workspacePath
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
	cmd.Dir = workspacePath
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
