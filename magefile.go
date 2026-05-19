//go:build mage

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	goimportsInstall    = "go install golang.org/x/tools/cmd/goimports@latest"
	gofumptInstall      = "go install mvdan.cc/gofumpt@latest"
	golangciLintInstall = "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.1"
	govulncheckInstall  = "go install golang.org/x/vuln/cmd/govulncheck@v1.3.0"

	buildDir       = "build"
	tmpDir         = "tmp"
	toolTmpDir     = tmpDir + "/.tmp"
	goTmpDir       = tmpDir + "/.go-build"
	mainPackage    = "./cmd/aila"
	binaryPath     = buildDir + "/aila"
	coverageOutput = tmpDir + "/coverage.out"
)

var Default = Check

// Fmt formats workspace Go source files with the project-approved formatter.
func Fmt() error {
	files, err := goFiles()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("skip fmt: no Go files yet")
		return nil
	}

	args := append([]string{"-w"}, files...)
	if _, err := exec.LookPath("goimports"); err == nil {
		return run("goimports", args...)
	}
	if _, err := exec.LookPath("gofumpt"); err == nil {
		return run("gofumpt", args...)
	}
	return fmt.Errorf("goimports or gofumpt not found; install one with `%s` or `%s`", goimportsInstall, gofumptInstall)
}

// Tidy normalizes module dependencies.
func Tidy() error {
	return run("go", "mod", "tidy")
}

// TidyCheck verifies go.mod and go.sum are already tidy without editing them.
func TidyCheck() error {
	return run("go", "mod", "tidy", "-diff")
}

// Test runs the Go test suite when packages exist.
func Test() error {
	if ok, err := hasPackages("test"); !ok || err != nil {
		return err
	}
	return run("go", "test", "./...")
}

// TestFast runs the Go test suite in short mode, skipping slow integration tests.
func TestFast() error {
	if ok, err := hasPackages("test:fast"); !ok || err != nil {
		return err
	}
	return run("go", "test", "-short", "./...")
}

// Coverage runs tests and writes the coverage profile under tmp/.
func Coverage() error {
	if ok, err := hasPackages("coverage"); !ok || err != nil {
		return err
	}
	if err := ensureDir(tmpDir); err != nil {
		return err
	}
	return run("go", "test", "-coverprofile", coverageOutput, "./...")
}

// Build compiles the aila binary into build/ when the main package exists.
func Build() error {
	if ok, err := hasMainPackage(); !ok || err != nil {
		return err
	}
	if err := ensureDir(buildDir); err != nil {
		return err
	}
	return run("go", "build", "-o", binaryPath, mainPackage)
}

// Vet runs go vet when packages exist.
func Vet() error {
	if ok, err := hasPackages("vet"); !ok || err != nil {
		return err
	}
	return run("go", "vet", "./...")
}

// Lint runs golangci-lint when packages exist.
func Lint() error {
	if ok, err := hasPackages("lint"); !ok || err != nil {
		return err
	}
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return missingTool("golangci-lint", golangciLintInstall)
	}
	return run("golangci-lint", "run", "./...")
}

// Vuln runs govulncheck when packages exist.
func Vuln() error {
	if ok, err := hasPackages("vuln"); !ok || err != nil {
		return err
	}
	if _, err := exec.LookPath("govulncheck"); err != nil {
		return missingTool("govulncheck", govulncheckInstall)
	}
	return run("govulncheck", "./...")
}

// Check runs all applicable verification gates.
func Check() error {
	steps := []struct {
		name string
		run  func() error
	}{
		{name: "tidy", run: TidyCheck},
		{name: "test", run: Test},
		{name: "vet", run: Vet},
		{name: "lint", run: Lint},
		{name: "vuln", run: Vuln},
	}

	for _, step := range steps {
		if err := step.run(); err != nil {
			return fmt.Errorf("%s failed: %w", step.name, err)
		}
	}
	return nil
}

func goFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}

		if entry.IsDir() {
			if path == "." {
				return nil
			}
			switch entry.Name() {
			case ".git", buildDir, tmpDir, "vendor":
				return filepath.SkipDir
			}
			return nil
		}

		if entry.Type().IsRegular() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func hasPackages(gate string) (bool, error) {
	command := exec.Command("go", "list", "./...")
	env, err := workspaceTempEnv()
	if err != nil {
		return false, err
	}
	command.Env = env
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return false, fmt.Errorf("go list ./... failed: %w: %s", err, message)
		}
		return false, fmt.Errorf("go list ./... failed: %w", err)
	}

	if strings.TrimSpace(string(out)) == "" {
		fmt.Printf("skip %s: no Go packages yet\n", gate)
		return false, nil
	}
	return true, nil
}

func hasMainPackage() (bool, error) {
	path := strings.TrimPrefix(mainPackage, "./")
	if _, err := os.Stat(filepath.FromSlash(path)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("skip build: %s not present yet\n", mainPackage)
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", mainPackage, err)
	}
	return true, nil
}

func missingTool(name string, install string) error {
	return fmt.Errorf("%s not found; install it with `%s`", name, install)
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	return nil
}

func workspaceTempEnv() ([]string, error) {
	if err := ensureDir(tmpDir); err != nil {
		return nil, err
	}
	tmpPath, err := filepath.Abs(toolTmpDir)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", toolTmpDir, err)
	}
	if err := os.MkdirAll(tmpPath, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", tmpPath, err)
	}
	goTmpPath, err := filepath.Abs(goTmpDir)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", goTmpDir, err)
	}
	if err := os.MkdirAll(goTmpPath, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", goTmpPath, err)
	}

	env := os.Environ()
	env = append(env, "TMPDIR="+tmpPath, "GOTMPDIR="+goTmpPath)
	return env, nil
}

func run(name string, args ...string) error {
	command := exec.Command(name, args...)
	env, err := workspaceTempEnv()
	if err != nil {
		return err
	}
	command.Env = env
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("%s exited with status %d", name, exitErr.ExitCode())
		}
		return err
	}
	return nil
}
