package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const storeDirName = ".aila"

const defaultProjectMetadata = "schema_version = 1\n"

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
	layout   Layout
	resolver Resolver
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
	if err := ctx.Err(); err != nil {
		return Store{}, err
	}

	layout, err := DescribeStore(workspacePath)
	if err != nil {
		return Store{}, err
	}
	if err := createLayout(ctx, layout); err != nil {
		return Store{}, err
	}

	return Store{
		layout:   layout,
		resolver: NewResolver(layout),
	}, nil
}

// Layout returns the store layout derived from the workspace path.
func (s Store) Layout() Layout {
	return s.layout
}

// Resolver returns the store resolver for logical artifact paths.
func (s Store) Resolver() Resolver {
	return s.resolver
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

func createLayout(ctx context.Context, layout Layout) error {
	for _, dir := range []string{layout.StoreRoot, layout.ArtifactsRoot, layout.IndexesRoot} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create store directory %s: %w", dir, err)
		}
	}
	return createProjectFile(layout.ProjectFile)
}

func createProjectFile(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			info, statErr := os.Stat(path)
			if statErr != nil {
				return fmt.Errorf("stat project metadata: %w", statErr)
			}
			if info.IsDir() {
				return fmt.Errorf("project metadata path is a directory: %s", path)
			}
			return nil
		}
		return fmt.Errorf("create project metadata: %w", err)
	}

	if _, err := file.WriteString(defaultProjectMetadata); err != nil {
		_ = file.Close()
		return fmt.Errorf("write project metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close project metadata: %w", err)
	}
	return nil
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
