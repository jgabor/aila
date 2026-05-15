package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDescribeStoreDerivesWorkspaceOwnedLayout(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	forbiddenRoots := []string{
		filepath.Join(t.TempDir(), "home"),
		filepath.Join(t.TempDir(), "xdg-state"),
		filepath.Join(t.TempDir(), "agentera"),
		filepath.Join(t.TempDir(), "tui"),
	}
	t.Setenv("HOME", forbiddenRoots[0])
	t.Setenv("XDG_STATE_HOME", forbiddenRoots[1])
	t.Setenv("AGENTERA_HOME", forbiddenRoots[2])
	t.Setenv("AILA_TUI_STATE", forbiddenRoots[3])

	layout, err := DescribeStore(workspace)
	if err != nil {
		t.Fatalf("DescribeStore returned error: %v", err)
	}

	wantStore := filepath.Join(workspace, ".aila")
	wants := map[string]string{
		"workspace root": workspace,
		"store root":     wantStore,
		"project file":   filepath.Join(wantStore, "project.toml"),
		"artifacts root": filepath.Join(wantStore, "artifacts"),
		"indexes root":   filepath.Join(wantStore, "indexes"),
	}
	got := map[string]string{
		"workspace root": layout.WorkspaceRoot,
		"store root":     layout.StoreRoot,
		"project file":   layout.ProjectFile,
		"artifacts root": layout.ArtifactsRoot,
		"indexes root":   layout.IndexesRoot,
	}
	for label, want := range wants {
		if got[label] != want {
			t.Fatalf("%s = %q, want %q", label, got[label], want)
		}
	}

	for _, forbidden := range forbiddenRoots {
		for label, path := range got {
			if path == forbidden || strings.HasPrefix(path, forbidden+string(filepath.Separator)) {
				t.Fatalf("%s path %q was derived from forbidden state root %q", label, path, forbidden)
			}
		}
	}
}

func TestResolveKnownArtifactIncludesPathAndProvenance(t *testing.T) {
	t.Parallel()

	layout := mustDescribeStore(t, filepath.Join(t.TempDir(), "workspace"))
	artifact, err := NewResolver(layout).ResolveArtifact(ArtifactProjectSummary)
	if err != nil {
		t.Fatalf("ResolveArtifact returned error: %v", err)
	}

	wantPath := filepath.Join(layout.ArtifactsRoot, "project-summary.md")
	if artifact.Path != wantPath {
		t.Fatalf("artifact path = %q, want %q", artifact.Path, wantPath)
	}
	assertPathInside(t, artifact.Path, layout.ArtifactsRoot)
	if artifact.Provenance.LogicalName != ArtifactProjectSummary {
		t.Fatalf("provenance logical name = %q, want %q", artifact.Provenance.LogicalName, ArtifactProjectSummary)
	}
	if artifact.Provenance.StoreRoot != layout.StoreRoot {
		t.Fatalf("provenance store root = %q, want %q", artifact.Provenance.StoreRoot, layout.StoreRoot)
	}
}

func TestResolveRejectsUnknownAndUnsafeArtifacts(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(mustDescribeStore(t, filepath.Join(t.TempDir(), "workspace")))

	if _, err := resolver.ResolveArtifact("missing_artifact"); !errors.Is(err, ErrUnknownArtifact) {
		t.Fatalf("unknown artifact error = %v, want ErrUnknownArtifact", err)
	}

	unsafeNames := []ArtifactName{
		"../project_summary",
		"project_summary/escape",
		"project-summary",
		" project_summary",
		"ProjectSummary",
		"",
	}
	for _, name := range unsafeNames {
		if _, err := resolver.ResolveArtifact(name); !errors.Is(err, ErrUnsafeArtifact) {
			t.Fatalf("unsafe artifact %q error = %v, want ErrUnsafeArtifact", name, err)
		}
	}
}

func TestResolveArtifactWriteRejectsUnownedWriteWithoutMutation(t *testing.T) {
	t.Parallel()

	layout := mustDescribeStore(t, filepath.Join(t.TempDir(), "workspace"))
	resolver := NewResolver(layout)
	artifact := mustResolveArtifact(t, resolver, ArtifactProjectSummary)

	if err := os.MkdirAll(filepath.Dir(artifact.Path), 0o755); err != nil {
		t.Fatalf("create artifact parent: %v", err)
	}
	const existing = "existing content"
	if err := os.WriteFile(artifact.Path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	if _, err := resolver.ResolveArtifactWrite(ArtifactProjectSummary, OwnerApp); !errors.Is(err, ErrUnauthorizedOwn) {
		t.Fatalf("unowned write error = %v, want ErrUnauthorizedOwn", err)
	}

	content, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatalf("read seeded artifact after rejected write: %v", err)
	}
	if string(content) != existing {
		t.Fatalf("rejected write mutated artifact content: got %q, want %q", content, existing)
	}
}

func TestResolveArtifactWriteAllowsOwningWriter(t *testing.T) {
	t.Parallel()

	layout := mustDescribeStore(t, filepath.Join(t.TempDir(), "workspace"))
	artifact, err := NewResolver(layout).ResolveArtifactWrite(ArtifactProjectSummary, OwnerState)
	if err != nil {
		t.Fatalf("ResolveArtifactWrite returned error: %v", err)
	}
	assertPathInside(t, artifact.Path, layout.ArtifactsRoot)
}

func TestOpenProjectStoreCreatesMinimalLayoutOnCleanWorkspace(t *testing.T) {
	t.Parallel()

	workspace := filepath.Join(t.TempDir(), "workspace")
	store, err := OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("OpenProjectStore returned error: %v", err)
	}
	layout := store.Layout()

	assertDir(t, layout.StoreRoot)
	assertDir(t, layout.ArtifactsRoot)
	assertDir(t, layout.IndexesRoot)
	assertFileContent(t, layout.ProjectFile, defaultProjectMetadata)
	assertStoreEntries(t, layout.StoreRoot, []string{"artifacts/", "indexes/", "project.toml"})
}

func TestOpenProjectStoreReopenPreservesExistingMetadata(t *testing.T) {
	t.Parallel()

	workspace := filepath.Join(t.TempDir(), "workspace")
	layout := mustDescribeStore(t, workspace)
	if err := os.MkdirAll(layout.StoreRoot, 0o755); err != nil {
		t.Fatalf("create store root: %v", err)
	}
	const metadata = "schema_version = 1\nproject_id = \"keep\"\n"
	if err := os.WriteFile(layout.ProjectFile, []byte(metadata), 0o644); err != nil {
		t.Fatalf("seed project metadata: %v", err)
	}

	if _, err := OpenProjectStore(context.Background(), workspace); err != nil {
		t.Fatalf("first OpenProjectStore returned error: %v", err)
	}
	store, err := OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("second OpenProjectStore returned error: %v", err)
	}

	assertFileContent(t, store.Layout().ProjectFile, metadata)
	assertStoreEntries(t, store.Layout().StoreRoot, []string{"artifacts/", "indexes/", "project.toml"})
}

func TestWriteArtifactStoresOwnedContentWithAtomicBoundary(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	artifact := mustResolveArtifact(t, store.Resolver(), ArtifactProjectSummary)
	if err := os.WriteFile(artifact.Path, []byte("old content\n"), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	written, err := store.WriteArtifact(context.Background(), ArtifactProjectSummary, OwnerState, []byte("complete content\n"))
	if err != nil {
		t.Fatalf("WriteArtifact returned error: %v", err)
	}
	if written.Path != artifact.Path {
		t.Fatalf("written artifact path = %q, want %q", written.Path, artifact.Path)
	}
	assertFileContent(t, artifact.Path, "complete content\n")
	assertNoTempArtifacts(t, filepath.Dir(artifact.Path))
}

func TestWriteArtifactRejectsUnownedWriteWithoutFinalMutation(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	artifact := mustResolveArtifact(t, store.Resolver(), ArtifactProjectSummary)

	if _, err := store.WriteArtifact(context.Background(), ArtifactProjectSummary, OwnerApp, []byte("bad content\n")); !errors.Is(err, ErrUnauthorizedOwn) {
		t.Fatalf("unowned write error = %v, want ErrUnauthorizedOwn", err)
	}
	if _, err := os.Stat(artifact.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unowned write created final artifact: stat error = %v", err)
	}

	const existing = "existing content\n"
	if err := os.WriteFile(artifact.Path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	if _, err := store.WriteArtifact(context.Background(), ArtifactProjectSummary, OwnerApp, []byte("bad content\n")); !errors.Is(err, ErrUnauthorizedOwn) {
		t.Fatalf("unowned rewrite error = %v, want ErrUnauthorizedOwn", err)
	}
	assertFileContent(t, artifact.Path, existing)
	assertNoTempArtifacts(t, filepath.Dir(artifact.Path))
}

func mustDescribeStore(t *testing.T, workspace string) Layout {
	t.Helper()
	layout, err := DescribeStore(workspace)
	if err != nil {
		t.Fatalf("DescribeStore(%q): %v", workspace, err)
	}
	return layout
}

func mustOpenProjectStore(t *testing.T, workspace string) Store {
	t.Helper()
	store, err := OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("OpenProjectStore(%q): %v", workspace, err)
	}
	return store
}

func mustResolveArtifact(t *testing.T, resolver Resolver, name ArtifactName) ResolvedArtifact {
	t.Helper()
	artifact, err := resolver.ResolveArtifact(name)
	if err != nil {
		t.Fatalf("ResolveArtifact(%q): %v", name, err)
	}
	return artifact
}

func assertPathInside(t *testing.T, path string, root string) {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("path %q relative to %q: %v", path, root, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatalf("path %q is not inside root %q", path, root)
	}
}

func assertDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat directory %q: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("content of %q = %q, want %q", path, content, want)
	}
}

func assertStoreEntries(t *testing.T, root string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read store root %q: %v", root, err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		got = append(got, name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("store entries = %v, want %v", got, want)
	}
}

func assertNoTempArtifacts(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read artifact root %q: %v", root, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".project-summary.md.") && strings.HasSuffix(name, ".tmp") {
			t.Fatalf("temporary artifact %q was left behind", name)
		}
	}
}
