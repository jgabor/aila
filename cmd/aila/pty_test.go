package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestStaticTUISmokeStartupAndQuit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	ctx, cancel, terminal, wait := startAilaPTY(t)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntil(t, terminal, "Aila", 20*time.Second)
	if !strings.Contains(startup, "Aila") {
		t.Fatalf("startup output missing Aila marker: %q", startup)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input: %v", err)
	}

	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("static TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("static TUI did not quit after q: %v", ctx.Err())
	}
}

func TestPromptSubmitPTYSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	ctx, cancel, terminal, wait := startAilaPTY(t)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("explain this repo\r")); err != nil {
		t.Fatalf("send prompt submit input: %v", err)
	}

	output := readUntil(t, terminal, "Fake Aila response: explain this repo", 10*time.Second)
	if !strings.Contains(output, "user: explain this repo") {
		t.Fatalf("submit output missing user prompt marker: %q", output)
	}
	if !strings.Contains(output, "assistant: Fake Aila response: explain this repo") {
		t.Fatalf("submit output missing assistant response marker: %q", output)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("prompt submit TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("prompt submit TUI did not quit after q: %v", ctx.Err())
	}
}

func startAilaPTY(t *testing.T) (context.Context, context.CancelFunc, *os.File, <-chan error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Env = ailaPTYEnv(t)

	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		cancel()
		t.Fatalf("start static TUI in PTY: %v", err)
	}

	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
	}()
	return ctx, cancel, terminal, wait
}

func ailaPTYEnv(t *testing.T) []string {
	t.Helper()

	tmp := t.TempDir()
	for _, dir := range []string{"home", "xdg-cache", "go-build", "tmp"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatalf("create PTY test environment directory: %v", err)
		}
	}

	env := []string{
		"TERM=xterm-256color",
		"HOME=" + filepath.Join(tmp, "home"),
		"XDG_CACHE_HOME=" + filepath.Join(tmp, "xdg-cache"),
		"GOCACHE=" + filepath.Join(tmp, "go-build"),
		"TMPDIR=" + filepath.Join(tmp, "tmp"),
		"GOENV=off",
	}
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	for _, name := range []string{"GOROOT", "GOMODCACHE"} {
		if value := goEnv(t, name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func goEnv(t *testing.T, name string) string {
	t.Helper()

	cmd := exec.Command("go", "env", name)
	cmd.Env = []string{"HOME=" + os.Getenv("HOME"), "GOENV=off"}
	if path := os.Getenv("PATH"); path != "" {
		cmd.Env = append(cmd.Env, "PATH="+path)
	}
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("read go env %s: %v", name, err)
	}
	return strings.TrimSpace(string(output))
}

func readUntil(t *testing.T, reader io.Reader, needle string, timeout time.Duration) string {
	t.Helper()

	result := make(chan string, 1)
	failure := make(chan error, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
				if strings.Contains(out.String(), needle) {
					result <- out.String()
					return
				}
			}
			if err != nil {
				failure <- fmt.Errorf("read PTY output: %w", err)
				return
			}
		}
	}()

	select {
	case output := <-result:
		return output
	case err := <-failure:
		t.Fatal(err)
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %q", needle)
	}
	return ""
}
