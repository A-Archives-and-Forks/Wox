package filesearch

import (
	"errors"
	"strings"
	"testing"
)

func TestWalkEverythingWithFallbackUsesLegacySDKWhenPrimaryReportsUnavailable(t *testing.T) {
	primaryCalled := false
	legacyCalled := false

	err := walkEverythingWithFallback(
		"session_index",
		20,
		func(path string, info FileInfo, err error) error { return nil },
		func(root string, maxCount int, walkFn WalkFunc) error {
			primaryCalled = true
			return ErrEverythingNotRunning
		},
		func(root string, maxCount int, walkFn WalkFunc) error {
			legacyCalled = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected legacy SDK fallback to succeed, got %v", err)
	}
	if !primaryCalled {
		t.Fatalf("expected primary SDK to be attempted")
	}
	if !legacyCalled {
		t.Fatalf("expected legacy SDK fallback to run")
	}
}

func TestWalkEverythingWithFallbackKeepsPrimaryErrorWhenFallbackIsNotApplicable(t *testing.T) {
	primaryErr := errors.New("primary search failed")
	legacyCalled := false

	err := walkEverythingWithFallback(
		"session_index",
		20,
		func(path string, info FileInfo, err error) error { return nil },
		func(root string, maxCount int, walkFn WalkFunc) error {
			return primaryErr
		},
		func(root string, maxCount int, walkFn WalkFunc) error {
			legacyCalled = true
			return nil
		},
	)
	if !errors.Is(err, primaryErr) {
		t.Fatalf("expected primary error to be returned, got %v", err)
	}
	if legacyCalled {
		t.Fatalf("expected legacy fallback to be skipped for non-availability errors")
	}
}

func TestNewEverything2QueryErrorOnlyMapsIPCFailureToNotRunning(t *testing.T) {
	err := newEverything2QueryError(87)
	if errors.Is(err, ErrEverythingNotRunning) {
		t.Fatalf("expected non-IPC failures to keep a distinct error, got %v", err)
	}
	if !strings.Contains(err.Error(), "last_error=87") {
		t.Fatalf("expected error to include last error code, got %q", err.Error())
	}
}

func TestNewEverything2QueryErrorMapsIPCFailureToNotRunning(t *testing.T) {
	err := newEverything2QueryError(everything2ErrorIPC)
	if !errors.Is(err, ErrEverythingNotRunning) {
		t.Fatalf("expected IPC failure to map to not running, got %v", err)
	}
}
