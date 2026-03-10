//go:build linux

package screenshot

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
