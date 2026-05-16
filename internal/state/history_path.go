package state

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var ErrUnsafeFakeHistoryPath = errors.New("fake history path is unsafe")

// FakeHistoryName identifies the single M17 fake activity history contract.
type FakeHistoryName string

const CurrentFakeHistory FakeHistoryName = "current_fake_history"

// FakeHistoryProvenance records how the fake history path was derived.
type FakeHistoryProvenance struct {
	LogicalName   FakeHistoryName
	WorkspaceRoot string
	StoreRoot     string
	RelativePath  string
}

// FakeHistoryLocation is the store-owned path for the current fake activity log.
type FakeHistoryLocation struct {
	Name       FakeHistoryName
	Path       string
	Provenance FakeHistoryProvenance
}

// DescribeFakeHistory derives `.aila/history/fake-events.jsonl` from the workspace store layout.
func DescribeFakeHistory(workspacePath string) (FakeHistoryLocation, error) {
	layout, err := DescribeStore(workspacePath)
	if err != nil {
		return FakeHistoryLocation{}, err
	}
	return CurrentFakeHistoryLocation(layout)
}

// CurrentFakeHistoryLocation derives the current fake history path from an existing store layout.
func CurrentFakeHistoryLocation(layout Layout) (FakeHistoryLocation, error) {
	const relativePath = "history/fake-events.jsonl"
	path := filepath.Clean(filepath.Join(layout.StoreRoot, relativePath))
	if err := ValidateFakeHistoryPath(layout, path); err != nil {
		return FakeHistoryLocation{}, err
	}
	return FakeHistoryLocation{
		Name: CurrentFakeHistory,
		Path: path,
		Provenance: FakeHistoryProvenance{
			LogicalName:   CurrentFakeHistory,
			WorkspaceRoot: layout.WorkspaceRoot,
			StoreRoot:     layout.StoreRoot,
			RelativePath:  relativePath,
		},
	}, nil
}

// ValidateFakeHistoryPath accepts only the exact fake history path in the store layout.
func ValidateFakeHistoryPath(layout Layout, requestedPath string) error {
	if strings.TrimSpace(requestedPath) == "" {
		return ErrUnsafeFakeHistoryPath
	}
	if containsParentTraversal(requestedPath) {
		return fmt.Errorf("%w: %s", ErrUnsafeFakeHistoryPath, requestedPath)
	}
	want := filepath.Clean(filepath.Join(layout.StoreRoot, "history", "fake-events.jsonl"))
	got := filepath.Clean(requestedPath)
	rel, err := filepath.Rel(layout.StoreRoot, got)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("%w: %s", ErrUnsafeFakeHistoryPath, requestedPath)
	}
	if got != want {
		return fmt.Errorf("%w: %s", ErrUnsafeFakeHistoryPath, requestedPath)
	}
	return nil
}
