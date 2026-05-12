package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("start static TUI in PTY: %v", err)
	}
	defer func() {
		_ = terminal.Close()
	}()

	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
	}()

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
