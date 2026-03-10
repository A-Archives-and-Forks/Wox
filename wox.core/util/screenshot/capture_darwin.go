//go:build darwin

package screenshot

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Cocoa -framework ApplicationServices
#include <stdlib.h>

typedef struct {
    unsigned char* data;
    int len;
    char* err;
} CapturePNGResult;

CapturePNGResult captureDisplayPNG(unsigned int displayID);
void releaseCapturePNGResult(CapturePNGResult result);
*/
import "C"
import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"strconv"
	"unsafe"
)

func (p *nativeCaptureProvider) ListDisplays() ([]Display, error) {
	return listDisplaysFromSystem()
}

func (p *nativeCaptureProvider) CaptureDisplays(displayIDs []string) (*image.RGBA, error) {
	displays, err := p.ListDisplays()
	if err != nil {
		return nil, err
	}

	selected := displays
	if len(displayIDs) > 0 {
		selected = filterDisplays(displays, displayIDs)
	}
	if len(selected) == 0 {
		return nil, ErrNoDisplays
	}

	bounds, err := getPixelBoundsForDisplays(displays, displayIDs)
	if err != nil {
		return nil, err
	}

	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Width, bounds.Height))
	for _, display := range selected {
		img, captureErr := captureDisplayImage(display.ID)
		if captureErr != nil {
			return nil, captureErr
		}

		destRect := image.Rect(
			display.PixelBounds.X-bounds.X,
			display.PixelBounds.Y-bounds.Y,
			display.PixelBounds.X-bounds.X+display.PixelBounds.Width,
			display.PixelBounds.Y-bounds.Y+display.PixelBounds.Height,
		)
		draw.Draw(canvas, destRect, img, img.Bounds().Min, draw.Src)
	}

	return canvas, nil
}

func (p *nativeCaptureProvider) CaptureRect(rect PixelRect) (*image.RGBA, error) {
	if rect.Width <= 0 || rect.Height <= 0 {
		return nil, fmt.Errorf("invalid capture rect: %+v", rect)
	}

	displays, err := p.ListDisplays()
	if err != nil {
		return nil, err
	}

	canvas := image.NewRGBA(image.Rect(0, 0, rect.Width, rect.Height))
	for _, display := range displays {
		intersection, ok := intersectPixelRect(display.PixelBounds, rect)
		if !ok {
			continue
		}

		img, captureErr := captureDisplayImage(display.ID)
		if captureErr != nil {
			return nil, captureErr
		}

		srcPoint := image.Point{
			X: intersection.X - display.PixelBounds.X,
			Y: intersection.Y - display.PixelBounds.Y,
		}
		destRect := image.Rect(
			intersection.X-rect.X,
			intersection.Y-rect.Y,
			intersection.X-rect.X+intersection.Width,
			intersection.Y-rect.Y+intersection.Height,
		)
		draw.Draw(canvas, destRect, img, srcPoint, draw.Src)
	}

	return canvas, nil
}

func captureDisplayImage(displayID string) (*image.RGBA, error) {
	id, err := strconv.ParseUint(displayID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid display id %q: %w", displayID, err)
	}

	result := C.captureDisplayPNG(C.uint(id))
	defer C.releaseCapturePNGResult(result)

	if result.err != nil {
		return nil, fmt.Errorf("capture display failed: %s", C.GoString(result.err))
	}
	if result.data == nil || result.len == 0 {
		return nil, fmt.Errorf("capture display returned empty image")
	}

	pngBytes := C.GoBytes(unsafe.Pointer(result.data), result.len)
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}

	return toRGBA(img), nil
}

func intersectPixelRect(a PixelRect, b PixelRect) (PixelRect, bool) {
	left := maxInt(a.X, b.X)
	top := maxInt(a.Y, b.Y)
	right := minInt(a.X+a.Width, b.X+b.Width)
	bottom := minInt(a.Y+a.Height, b.Y+b.Height)
	if right <= left || bottom <= top {
		return PixelRect{}, false
	}

	return PixelRect{
		X:      left,
		Y:      top,
		Width:  right - left,
		Height: bottom - top,
	}, true
}
