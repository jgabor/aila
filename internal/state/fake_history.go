package state

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/history"
)

const (
	FakeHistoryMaxFileBytes = 256 * 1024
	FakeHistoryMaxLineBytes = 8 * 1024
)

var writeFakeHistoryFileAtomic = writeFileAtomic

// FakeHistoryReadState reports whether the fake history log loaded or needs passive recovery.
type FakeHistoryReadState string

const (
	FakeHistoryEmpty          FakeHistoryReadState = "empty"
	FakeHistoryLoaded         FakeHistoryReadState = "loaded"
	FakeHistoryRecoveryNeeded FakeHistoryReadState = "recovery_needed"
)

// FakeHistoryReadResult carries validated fake history events or a passive recovery diagnostic.
type FakeHistoryReadResult struct {
	State       FakeHistoryReadState
	Events      []history.FakeEvent
	Diagnostics []diagnostic.Diagnostic
}

// FakeHistoryAppendResult reports an append success or a passive recovery diagnostic.
type FakeHistoryAppendResult struct {
	State       FakeHistoryReadState
	Location    FakeHistoryLocation
	Diagnostics []diagnostic.Diagnostic
}

// ReadFakeHistory reads and validates `.aila/history/fake-events.jsonl` without repairing it.
func (s Store) ReadFakeHistory(ctx context.Context) (FakeHistoryReadResult, error) {
	if err := ctx.Err(); err != nil {
		return FakeHistoryReadResult{}, err
	}
	location, err := CurrentFakeHistoryLocation(s.layout)
	if err != nil {
		return FakeHistoryReadResult{}, err
	}
	if err := validateFakeHistoryComponents(s.layout, location.Path); err != nil {
		return fakeHistoryRecoveryResult(err), nil
	}

	info, err := os.Lstat(location.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FakeHistoryReadResult{State: FakeHistoryEmpty}, nil
		}
		return fakeHistoryRecoveryResult(fmt.Errorf("stat fake history: %w", err)), nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fakeHistoryRecoveryResult(fmt.Errorf("%w: fake history is a symlink", ErrUnsafeFakeHistoryPath)), nil
	}
	if info.IsDir() {
		return fakeHistoryRecoveryResult(fmt.Errorf("fake history path is a directory")), nil
	}
	if info.Size() == 0 {
		return FakeHistoryReadResult{State: FakeHistoryEmpty}, nil
	}
	if info.Size() > FakeHistoryMaxFileBytes {
		return fakeHistoryRecoveryResult(fmt.Errorf("fake history exceeds %d bytes", FakeHistoryMaxFileBytes)), nil
	}

	events, err := readFakeHistoryEvents(location.Path)
	if err != nil {
		return fakeHistoryRecoveryResult(err), nil
	}
	if len(events) == 0 {
		return FakeHistoryReadResult{State: FakeHistoryEmpty}, nil
	}
	return FakeHistoryReadResult{State: FakeHistoryLoaded, Events: events}, nil
}

// AppendFakeHistory validates, redacts, and appends one fake event to `.aila/history/fake-events.jsonl`.
func (s Store) AppendFakeHistory(ctx context.Context, event history.FakeEvent) (FakeHistoryAppendResult, error) {
	if err := ctx.Err(); err != nil {
		return FakeHistoryAppendResult{}, err
	}
	location, err := CurrentFakeHistoryLocation(s.layout)
	if err != nil {
		return FakeHistoryAppendResult{}, err
	}
	normalized, err := history.NormalizeFakeEvent(event)
	if err != nil {
		return FakeHistoryAppendResult{}, err
	}
	line, err := encodeFakeHistoryLine(normalized)
	if err != nil {
		return FakeHistoryAppendResult{}, err
	}

	if err := validateFakeHistoryComponents(s.layout, location.Path); err != nil {
		return fakeHistoryAppendRecoveryResult(location, err), nil
	}
	if existing, err := s.ReadFakeHistory(ctx); err != nil {
		return FakeHistoryAppendResult{}, err
	} else if existing.State == FakeHistoryRecoveryNeeded {
		return FakeHistoryAppendResult{State: FakeHistoryRecoveryNeeded, Location: location, Diagnostics: existing.Diagnostics}, nil
	}
	if err := ensureFakeHistoryDirectory(s.layout, location.Path); err != nil {
		return FakeHistoryAppendResult{}, err
	}
	if err := validateFakeHistoryComponents(s.layout, location.Path); err != nil {
		return fakeHistoryAppendRecoveryResult(location, err), nil
	}
	if err := validateFakeHistoryFinalFile(location.Path); err != nil {
		return fakeHistoryAppendRecoveryResult(location, err), nil
	}
	if err := validateFakeHistoryAppendBound(location.Path, len(line)); err != nil {
		return fakeHistoryAppendRecoveryResult(location, err), nil
	}
	if err := appendFakeHistoryLine(ctx, location.Path, line); err != nil {
		return FakeHistoryAppendResult{}, err
	}
	return FakeHistoryAppendResult{State: FakeHistoryLoaded, Location: location}, nil
}

func readFakeHistoryEvents(path string) ([]history.FakeEvent, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fake history: %w", err)
	}
	if len(content) == 0 {
		return nil, nil
	}
	if !bytes.HasSuffix(content, []byte("\n")) {
		return nil, fmt.Errorf("fake history has partial final event")
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 1024), FakeHistoryMaxLineBytes+1)
	events := make([]history.FakeEvent, 0)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			return nil, fmt.Errorf("fake history line %d is empty", lineNumber)
		}
		event, err := decodeFakeHistoryLine(line)
		if err != nil {
			return nil, fmt.Errorf("fake history line %d: %w", lineNumber, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan fake history: %w", err)
	}
	return events, nil
}

func decodeFakeHistoryLine(line []byte) (history.FakeEvent, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	var event history.FakeEvent
	if err := decoder.Decode(&event); err != nil {
		return history.FakeEvent{}, fmt.Errorf("decode event: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return history.FakeEvent{}, fmt.Errorf("decode event: trailing data")
	}
	normalized, err := history.NormalizeFakeEvent(event)
	if err != nil {
		return history.FakeEvent{}, err
	}
	return normalized, nil
}

func encodeFakeHistoryLine(event history.FakeEvent) ([]byte, error) {
	content, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("encode fake history event: %w", err)
	}
	if len(content)+1 > FakeHistoryMaxLineBytes {
		return nil, fmt.Errorf("%w: encoded event exceeds %d bytes", history.ErrInvalidFakeEvent, FakeHistoryMaxLineBytes)
	}
	return append(content, '\n'), nil
}

func appendFakeHistoryLine(ctx context.Context, path string, line []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read fake history before append: %w", err)
		}
		existing = nil
	}
	if len(existing)+len(line) > FakeHistoryMaxFileBytes {
		return fmt.Errorf("fake history append would exceed %d bytes", FakeHistoryMaxFileBytes)
	}
	content := make([]byte, 0, len(existing)+len(line))
	content = append(content, existing...)
	content = append(content, line...)
	if err := writeFakeHistoryFileAtomic(ctx, path, content); err != nil {
		return fmt.Errorf("append fake history event atomically: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func validateFakeHistoryComponents(layout Layout, path string) error {
	if err := ValidateFakeHistoryPath(layout, path); err != nil {
		return err
	}
	for _, dir := range []string{layout.StoreRoot, filepath.Dir(path)} {
		info, err := os.Lstat(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("%w: inspect %s: %w", ErrUnsafeFakeHistoryPath, dir, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s is a symlink", ErrUnsafeFakeHistoryPath, dir)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %s is not a directory", ErrUnsafeFakeHistoryPath, dir)
		}
	}
	return nil
}

func ensureFakeHistoryDirectory(layout Layout, path string) error {
	dir := filepath.Dir(path)
	if err := ValidateFakeHistoryPath(layout, path); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create fake history directory: %w", err)
	}
	return nil
}

func validateFakeHistoryFinalFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%w: inspect fake history: %w", ErrUnsafeFakeHistoryPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: fake history is a symlink", ErrUnsafeFakeHistoryPath)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: fake history is a directory", ErrUnsafeFakeHistoryPath)
	}
	return nil
}

func validateFakeHistoryAppendBound(path string, lineBytes int) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect fake history size: %w", err)
	}
	if info.Size()+int64(lineBytes) > FakeHistoryMaxFileBytes {
		return fmt.Errorf("fake history append would exceed %d bytes", FakeHistoryMaxFileBytes)
	}
	return nil
}

func fakeHistoryRecoveryResult(cause error) FakeHistoryReadResult {
	return FakeHistoryReadResult{
		State:       FakeHistoryRecoveryNeeded,
		Diagnostics: []diagnostic.Diagnostic{fakeHistoryRecoveryDiagnostic(cause)},
	}
}

func fakeHistoryAppendRecoveryResult(location FakeHistoryLocation, cause error) FakeHistoryAppendResult {
	return FakeHistoryAppendResult{
		State:       FakeHistoryRecoveryNeeded,
		Location:    location,
		Diagnostics: []diagnostic.Diagnostic{fakeHistoryRecoveryDiagnostic(cause)},
	}
}

func fakeHistoryRecoveryDiagnostic(cause error) diagnostic.Diagnostic {
	return diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateHistory,
		Severity:         diagnostic.SeverityError,
		Message:          "fake history requires recovery: " + cause.Error(),
		AffectedArtifact: diagnostic.ArtifactFakeHistory,
		RecoveryAction:   diagnostic.RecoveryManualRepair,
		UserInputNeeded:  true,
	})
}
