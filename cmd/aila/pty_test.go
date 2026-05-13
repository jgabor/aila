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

func TestM5CommandPTYSmoke(t *testing.T) {
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

	if _, err := terminal.Write([]byte{0x18, 's'}); err != nil {
		t.Fatalf("send ctrl+x s command input: %v", err)
	}
	shortcutStatus := readUntil(t, terminal, "real status sources: deferred", 10*time.Second)
	if !strings.Contains(startup+shortcutStatus, "80x24") {
		t.Fatalf("PTY smoke did not observe fixed-size marker: startup=%q status=%q", startup, shortcutStatus)
	}
	for _, marker := range []string{
		"status:",
		"command route: status",
		"route source: policy.command",
		"Deterministic placeholder status.",
		"real status sources: deferred",
	} {
		if !strings.Contains(shortcutStatus, marker) {
			t.Fatalf("ctrl+x s output missing explicit marker %q: %q", marker, shortcutStatus)
		}
	}

	if _, err := terminal.Write([]byte("/help\r\n")); err != nil {
		t.Fatalf("send /help command input: %v", err)
	}
	readUntil(t, terminal, "Deterministic placeholder help.", 10*time.Second)

	if _, err := terminal.Write([]byte("/status\r\n")); err != nil {
		t.Fatalf("send /status command input: %v", err)
	}
	status := readUntil(t, terminal, "real status sources: deferred", 10*time.Second)
	for _, marker := range []string{
		"status:",
		"command route: status",
		"Deterministic placeholder status.",
		"real status sources: deferred",
	} {
		if !strings.Contains(status, marker) {
			t.Fatalf("/status output missing explicit marker %q: %q", marker, status)
		}
	}

	if _, err := terminal.Write([]byte("/quit\r\n")); err != nil {
		t.Fatalf("send /quit command input: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("/quit command route returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("/quit command route did not clean up before timeout: %v", ctx.Err())
	}
}

func TestM6ResizePTYSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	ctx, cancel, terminal, wait := startAilaPTYWithSize(t, 160, 45)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntil(t, terminal, "160x45", 20*time.Second)
	if !strings.Contains(startup, "Aila") {
		t.Fatalf("wide startup output missing Aila marker: %q", startup)
	}

	if _, err := terminal.Write([]byte("resize smoke\r")); err != nil {
		t.Fatalf("send resize smoke prompt input: %v", err)
	}
	readUntil(t, terminal, "assistant: Fake Aila response: resize smoke", 10*time.Second)

	if err := pty.Setsize(terminal, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("resize PTY to 80x24: %v", err)
	}
	resized := readUntilAll(t, terminal, []string{
		"Aila",
		"80x24",
		"Conversation",
		"user: resize smoke",
		"assistant: Fake Aila response: resize smoke",
		"Prompt",
		">",
		"git: placeholder | context: placeholder | q quit",
	}, 10*time.Second)
	if strings.Contains(resized, "Session") {
		t.Fatalf("80x24 resize output exposed right rail: %q", resized)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after resize: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("resize TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("resize TUI did not clean up before timeout: %v", ctx.Err())
	}
}

func TestM8DisplayLabelsPTYSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 80, 24, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntilAll(t, terminal, []string{
		"Aila",
		"primary model: opencode-go/deepseek-v4-pro:high",
		"utility model: opencode-go/deepseek-v4-flash:max",
		"autonomy: yolo (display-only)",
	}, 20*time.Second)
	for _, forbidden := range []string{"OPENAI", "ANTHROPIC", "GOOGLE_API", "config.toml", ".config/aila"} {
		if strings.Contains(startup, forbidden) {
			t.Fatalf("scrubbed PTY startup exposed config or credential marker %q: %q", forbidden, startup)
		}
	}
	configPath := filepath.Join(env.xdgConfigHome, "aila", "config.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read temp XDG config created by PTY startup: %v", err)
	}
	configText := string(config)
	for _, marker := range []string{
		`model = "opencode-go/deepseek-v4-pro:high"`,
		`[llm.utility]`,
		`model = "opencode-go/deepseek-v4-flash:max"`,
		`[autonomy]`,
		`level = "yolo"`,
	} {
		if !strings.Contains(configText, marker) {
			t.Fatalf("temp XDG config missing default marker %q: %s", marker, configText)
		}
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("display labels TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("display labels TUI did not quit after q: %v", ctx.Err())
	}
}

func startAilaPTY(t *testing.T) (context.Context, context.CancelFunc, *os.File, <-chan error) {
	t.Helper()
	return startAilaPTYWithSize(t, 80, 24)
}

func startAilaPTYWithSize(t *testing.T, cols uint16, rows uint16) (context.Context, context.CancelFunc, *os.File, <-chan error) {
	t.Helper()
	return startAilaPTYWithSizeAndEnv(t, cols, rows, newAilaPTYEnv(t).vars)
}

func startAilaPTYWithSizeAndEnv(t *testing.T, cols uint16, rows uint16, env []string) (context.Context, context.CancelFunc, *os.File, <-chan error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Env = env

	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
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

type ailaPTYTestEnv struct {
	vars          []string
	xdgConfigHome string
}

func newAilaPTYEnv(t *testing.T) ailaPTYTestEnv {
	t.Helper()

	tmp := t.TempDir()
	for _, dir := range []string{"home", "xdg-cache", "xdg-config", "go-build", "tmp"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatalf("create PTY test environment directory: %v", err)
		}
	}
	xdgConfigHome := filepath.Join(tmp, "xdg-config")

	env := []string{
		"TERM=xterm-256color",
		"HOME=" + filepath.Join(tmp, "home"),
		"XDG_CONFIG_HOME=" + xdgConfigHome,
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
	return ailaPTYTestEnv{vars: env, xdgConfigHome: xdgConfigHome}
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
	return readUntilMatch(t, reader, timeout, func(output string) bool {
		return strings.Contains(output, needle)
	}, needle)
}

func readUntilAll(t *testing.T, reader io.Reader, needles []string, timeout time.Duration) string {
	t.Helper()
	return readUntilMatch(t, reader, timeout, func(output string) bool {
		for _, needle := range needles {
			if !strings.Contains(output, needle) {
				return false
			}
		}
		return true
	}, strings.Join(needles, ", "))
}

func readUntilMatch(t *testing.T, reader io.Reader, timeout time.Duration, match func(string) bool, description string) string {
	t.Helper()

	result := make(chan string, 1)
	failure := make(chan error, 1)
	progress := make(chan string, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
				select {
				case progress <- out.String():
				default:
				}
				if match(out.String()) {
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

	var partial string
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case output := <-result:
			return output
		case err := <-failure:
			t.Fatal(err)
		case partial = <-progress:
		case <-timer.C:
			t.Fatalf("timed out waiting for %q; partial output: %q", description, partial)
		}
	}
}
