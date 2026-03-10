//go:build linux

package screenshot

import "image"

func (p *nativeCaptureProvider) ListDisplays() ([]Display, error) {
	return listDisplaysFromSystem()
}

func (p *nativeCaptureProvider) CaptureDisplays(displayIDs []string) (*image.RGBA, error) {
	_ = displayIDs
	return nil, ErrNotImplemented
}

func (p *nativeCaptureProvider) CaptureRect(rect PixelRect) (*image.RGBA, error) {
	_ = rect
	return nil, ErrNotImplemented
}
