//go:build windows

package screenshot

/*
#cgo CFLAGS: -DUNICODE -D_UNICODE
#cgo LDFLAGS: -lgdi32 -luser32
*/
import "C"

type nativeShell struct{}

type nativeCaptureProvider struct{}

func newShell() Shell {
	return &nativeShell{}
}

func newCaptureProvider() CaptureProvider {
	return &nativeCaptureProvider{}
}

func (s *nativeShell) Run(sessionID string, initial ViewModel, handler EventHandler) error {
	_ = sessionID
	_ = initial
	_ = handler
	return ErrNotImplemented
}

func (s *nativeShell) Update(view ViewModel) error {
	_ = view
	return ErrNotImplemented
}

func (s *nativeShell) Close() error {
	return nil
}
