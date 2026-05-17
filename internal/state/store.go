package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/jgabor/aila/internal/diagnostic"
)

const storeDirName = ".aila"

const defaultProjectMetadata = "schema_version = 1\n"

const projectMetadataSchemaVersion = 1

var (
	ErrEmptyWorkspace  = errors.New("workspace path is empty")
	ErrUnsafeArtifact  = errors.New("logical artifact name is unsafe")
	ErrUnknownArtifact = errors.New("logical artifact name is unknown")
	ErrUnauthorizedOwn = errors.New("artifact owner is not allowed")
	ErrUnsafeStorePath = errors.New("artifact path escapes store layout")
)

// Layout describes the minimal M15 project-visible store layout without creating it.
type Layout struct {
	WorkspaceRoot string
	StoreRoot     string
	ProjectFile   string
	ArtifactsRoot string
	IndexesRoot   string
}

// ArtifactName is a logical name from the store-owned artifact catalog.
type ArtifactName string

const (
	ArtifactProjectSummary ArtifactName = "project_summary"
	ArtifactPlan           ArtifactName = "plan"
	ArtifactVision         ArtifactName = "vision"
	ArtifactDecisions      ArtifactName = "decisions"
	ArtifactProfile        ArtifactName = "profile"
)

// ArtifactOwner identifies the invoking owner requesting an artifact write.
type ArtifactOwner string

const (
	OwnerState ArtifactOwner = "state"
	OwnerApp   ArtifactOwner = "app"
)

// Provenance records why a resolved path exists.
type Provenance struct {
	LogicalName ArtifactName
	StoreRoot   string
}

// ResolvedArtifact is a path derived from the workspace store layout.
type ResolvedArtifact struct {
	Name       ArtifactName
	Path       string
	Provenance Provenance
}

// Resolver resolves logical artifact names inside a described store layout.
type Resolver struct {
	layout Layout
}

// Store owns project-visible state layout creation and artifact writes.
type Store struct {
	layout     Layout
	resolver   Resolver
	openStatus OpenStatus
}

// OpenState describes whether a store open is ready for normal use or degraded.
type OpenState string

const (
	OpenStateInitialized    OpenState = "initialized"
	OpenStateRecoveryNeeded OpenState = "recovery_needed"
)

// OpenStatus reports passive startup state discovered while opening the store.
type OpenStatus struct {
	State       OpenState
	Diagnostics []diagnostic.Diagnostic
}

type artifactSpec struct {
	relPath string
	owners  map[ArtifactOwner]bool
}

var artifactCatalog = map[ArtifactName]artifactSpec{
	ArtifactProjectSummary: {
		relPath: "project-summary.md",
		owners: map[ArtifactOwner]bool{
			OwnerState: true,
		},
	},
	ArtifactPlan: {
		relPath: "plan.md",
		owners: map[ArtifactOwner]bool{
			OwnerApp: true,
		},
	},
	ArtifactVision: {
		relPath: "vision.md",
		owners: map[ArtifactOwner]bool{
			OwnerApp: true,
		},
	},
	ArtifactDecisions: {
		relPath: "decisions.md",
		owners: map[ArtifactOwner]bool{
			OwnerApp: true,
		},
	},
	ArtifactProfile: {
		relPath: "profile.md",
		owners: map[ArtifactOwner]bool{
			OwnerApp: true,
		},
	},
}

// DescribeStore derives the minimal M15 store paths from the provided workspace path.
func DescribeStore(workspacePath string) (Layout, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return Layout{}, ErrEmptyWorkspace
	}

	workspaceRoot := filepath.Clean(workspacePath)
	storeRoot := filepath.Join(workspaceRoot, storeDirName)
	return Layout{
		WorkspaceRoot: workspaceRoot,
		StoreRoot:     storeRoot,
		ProjectFile:   filepath.Join(storeRoot, "project.toml"),
		ArtifactsRoot: filepath.Join(storeRoot, "artifacts"),
		IndexesRoot:   filepath.Join(storeRoot, "indexes"),
	}, nil
}

// OpenProjectStore creates or reopens the minimal M15 project store layout.
func OpenProjectStore(ctx context.Context, workspacePath string) (Store, error) {
	result, err := OpenProjectStoreWithStatus(ctx, workspacePath)
	if err != nil {
		return Store{}, err
	}
	return result.Store, nil
}

// OpenProjectStoreWithStatus creates or reopens the project store and reports passive recovery state.
func OpenProjectStoreWithStatus(ctx context.Context, workspacePath string) (OpenResult, error) {
	if err := ctx.Err(); err != nil {
		return OpenResult{}, err
	}

	layout, err := DescribeStore(workspacePath)
	if err != nil {
		return OpenResult{}, err
	}
	status, err := createLayout(ctx, layout)
	if err != nil {
		return OpenResult{}, err
	}

	store := Store{
		layout:     layout,
		resolver:   NewResolver(layout),
		openStatus: status,
	}
	return OpenResult{Store: store, Status: status}, nil
}

// OpenResult returns both the usable store handles and passive recovery state.
type OpenResult struct {
	Store  Store
	Status OpenStatus
}

// Layout returns the store layout derived from the workspace path.
func (s Store) Layout() Layout {
	return s.layout
}

// Resolver returns the store resolver for logical artifact paths.
func (s Store) Resolver() Resolver {
	return s.resolver
}

// OpenStatus returns the passive startup state discovered while opening the store.
func (s Store) OpenStatus() OpenStatus {
	return s.openStatus
}

// WriteArtifact validates ownership and atomically replaces a logical artifact file.
func (s Store) WriteArtifact(ctx context.Context, name ArtifactName, owner ArtifactOwner, content []byte) (ResolvedArtifact, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedArtifact{}, err
	}

	artifact, err := s.resolver.ResolveArtifactWrite(name, owner)
	if err != nil {
		return ResolvedArtifact{}, err
	}
	if err := os.MkdirAll(filepath.Dir(artifact.Path), 0o755); err != nil {
		return ResolvedArtifact{}, fmt.Errorf("create artifact directory: %w", err)
	}
	if err := writeFileAtomic(ctx, artifact.Path, content); err != nil {
		return ResolvedArtifact{}, err
	}

	return artifact, nil
}

// NewResolver returns a pure resolver for a previously described layout.
func NewResolver(layout Layout) Resolver {
	return Resolver{layout: layout}
}

// ResolveArtifact resolves a known logical artifact name into the artifacts layout.
func (r Resolver) ResolveArtifact(name ArtifactName) (ResolvedArtifact, error) {
	spec, err := catalogSpec(name)
	if err != nil {
		return ResolvedArtifact{}, err
	}
	return r.resolved(name, spec)
}

// ResolveArtifactWrite validates ownership before returning a final artifact path for writing.
func (r Resolver) ResolveArtifactWrite(name ArtifactName, owner ArtifactOwner) (ResolvedArtifact, error) {
	spec, err := catalogSpec(name)
	if err != nil {
		return ResolvedArtifact{}, err
	}
	if !spec.owners[owner] {
		return ResolvedArtifact{}, fmt.Errorf("%w: %s cannot write %s", ErrUnauthorizedOwn, owner, name)
	}
	return r.resolved(name, spec)
}

func createLayout(ctx context.Context, layout Layout) (OpenStatus, error) {
	for _, dir := range []string{layout.StoreRoot, layout.ArtifactsRoot, layout.IndexesRoot} {
		if err := ctx.Err(); err != nil {
			return OpenStatus{}, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return OpenStatus{}, fmt.Errorf("create store directory %s: %w", dir, err)
		}
	}
	return createProjectFile(layout.ProjectFile)
}

func createProjectFile(path string) (OpenStatus, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			info, statErr := os.Stat(path)
			if statErr != nil {
				return OpenStatus{}, fmt.Errorf("stat project metadata: %w", statErr)
			}
			if info.IsDir() {
				return OpenStatus{}, fmt.Errorf("project metadata path is a directory: %s", path)
			}
			return validateProjectMetadata(path)
		}
		return OpenStatus{}, fmt.Errorf("create project metadata: %w", err)
	}

	if _, err := file.WriteString(defaultProjectMetadata); err != nil {
		_ = file.Close()
		return OpenStatus{}, fmt.Errorf("write project metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return OpenStatus{}, fmt.Errorf("close project metadata: %w", err)
	}
	return initializedStatus(), nil
}

func validateProjectMetadata(path string) (OpenStatus, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return OpenStatus{}, fmt.Errorf("read project metadata: %w", err)
	}
	version, err := parseProjectMetadataSchemaVersion(string(content))
	if err != nil {
		return recoveryStatus(err), nil
	}
	if version != projectMetadataSchemaVersion {
		return recoveryStatus(fmt.Errorf("unsupported schema_version %d", version)), nil
	}
	return initializedStatus(), nil
}

func parseProjectMetadataSchemaVersion(content string) (int, error) {
	seen := false
	version := 0
	for lineNumber, line := range strings.Split(content, "\n") {
		trimmed, err := stripTOMLComment(line)
		if err != nil {
			return 0, fmt.Errorf("invalid project metadata line %d", lineNumber+1)
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			if err := validateTOMLTable(trimmed); err != nil {
				return 0, fmt.Errorf("invalid project metadata line %d", lineNumber+1)
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			return 0, fmt.Errorf("invalid project metadata line %d", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !validTOMLKeyPath(key) || !validTOMLValue(value) {
			return 0, fmt.Errorf("invalid project metadata line %d", lineNumber+1)
		}
		if key != "schema_version" {
			continue
		}
		if seen {
			return 0, fmt.Errorf("duplicate schema_version")
		}
		seen = true
		parsedVersion, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("invalid schema_version")
		}
		version = parsedVersion
	}
	if seen {
		return version, nil
	}
	return 0, fmt.Errorf("missing schema_version")
}

func stripTOMLComment(line string) (string, error) {
	inString := false
	escaped := false
	for i, r := range line {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '#':
			return strings.TrimSpace(line[:i]), nil
		}
	}
	if inString || escaped {
		return "", fmt.Errorf("unterminated string")
	}
	return strings.TrimSpace(line), nil
}

func validateTOMLTable(line string) error {
	if !strings.HasSuffix(line, "]") || strings.HasPrefix(line, "[[") || strings.Contains(line[1:len(line)-1], "[") || strings.Contains(line[1:len(line)-1], "]") {
		return fmt.Errorf("invalid table")
	}
	if !validTOMLKeyPath(strings.TrimSpace(line[1 : len(line)-1])) {
		return fmt.Errorf("invalid table")
	}
	return nil
}

func validTOMLKeyPath(key string) bool {
	if key == "" {
		return false
	}
	parts := strings.Split(key, ".")
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func validTOMLValue(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "\"") {
		_, err := strconv.Unquote(value)
		return err == nil
	}
	if value == "true" || value == "false" {
		return true
	}
	if _, err := strconv.Atoi(value); err == nil {
		return true
	}
	return false
}

func initializedStatus() OpenStatus {
	return OpenStatus{State: OpenStateInitialized}
}

func recoveryStatus(cause error) OpenStatus {
	return OpenStatus{
		State: OpenStateRecoveryNeeded,
		Diagnostics: []diagnostic.Diagnostic{diagnostic.New(diagnostic.Spec{
			Category:         diagnostic.CategoryState,
			Source:           diagnostic.SourceStateOpen,
			Severity:         diagnostic.SeverityError,
			Message:          "project metadata requires recovery: " + cause.Error(),
			AffectedArtifact: diagnostic.ArtifactProjectMetadata,
			RecoveryAction:   diagnostic.RecoveryManualRepair,
			UserInputNeeded:  true,
		})},
	}
}

func writeFileAtomic(ctx context.Context, path string, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create artifact temp file: %w", err)
	}
	tempPath := file.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write artifact temp file: %w", err)
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return fmt.Errorf("chmod artifact temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close artifact temp file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace artifact file: %w", err)
	}

	removeTemp = false
	return nil
}

func catalogSpec(name ArtifactName) (artifactSpec, error) {
	if !safeArtifactName(name) {
		return artifactSpec{}, fmt.Errorf("%w: %q", ErrUnsafeArtifact, name)
	}
	spec, ok := artifactCatalog[name]
	if !ok {
		return artifactSpec{}, fmt.Errorf("%w: %s", ErrUnknownArtifact, name)
	}
	return spec, nil
}

func (r Resolver) resolved(name ArtifactName, spec artifactSpec) (ResolvedArtifact, error) {
	path := filepath.Clean(filepath.Join(r.layout.ArtifactsRoot, spec.relPath))
	rel, err := filepath.Rel(r.layout.ArtifactsRoot, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return ResolvedArtifact{}, fmt.Errorf("%w: %s", ErrUnsafeStorePath, path)
	}
	return ResolvedArtifact{
		Name: name,
		Path: path,
		Provenance: Provenance{
			LogicalName: name,
			StoreRoot:   r.layout.StoreRoot,
		},
	}, nil
}

func safeArtifactName(name ArtifactName) bool {
	value := string(name)
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}
