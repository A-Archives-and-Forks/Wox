package screenshot

import "errors"

var (
	ErrNotImplemented   = errors.New("not implemented")
	ErrSessionClosed    = errors.New("session closed")
	ErrSessionCancelled = errors.New("session cancelled")
	ErrSessionNotFound  = errors.New("session not found")
	ErrNoDisplays       = errors.New("no displays available")
	ErrEmptySelection   = errors.New("selection is empty")
	ErrNoImage          = errors.New("image is nil")
)
