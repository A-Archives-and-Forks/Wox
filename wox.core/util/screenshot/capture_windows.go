//go:build windows

package screenshot

/*
#cgo CFLAGS: -DUNICODE -D_UNICODE
#cgo LDFLAGS: -lgdi32 -luser32
#include <stdlib.h>

typedef struct {
    unsigned char* data;
    int len;
    int width;
    int height;
    char* err;
} CaptureRawResult;

CaptureRawResult captureRectBGRA(int x, int y, int width, int height);
void releaseCaptureRawResult(CaptureRawResult result);
*/
import "C"
import (
	"fmt"
	"image"
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

	rect, err := getPixelBoundsForDisplays(displays, displayIDs)
	if err != nil {
		return nil, err
	}

	return p.CaptureRect(rect)
}

func (p *nativeCaptureProvider) CaptureRect(rect PixelRect) (*image.RGBA, error) {
	if rect.Width <= 0 || rect.Height <= 0 {
		return nil, fmt.Errorf("invalid capture rect: %+v", rect)
	}

	result := C.captureRectBGRA(C.int(rect.X), C.int(rect.Y), C.int(rect.Width), C.int(rect.Height))
	defer C.releaseCaptureRawResult(result)

	if result.err != nil {
		return nil, fmt.Errorf("capture rect failed: %s", C.GoString(result.err))
	}
	if result.data == nil || result.len == 0 {
		return nil, fmt.Errorf("capture rect returned empty buffer")
	}

	return bgraToRGBA(
		C.GoBytes(unsafe.Pointer(result.data), result.len),
		int(result.width),
		int(result.height),
	), nil
}

func bgraToRGBA(src []byte, width int, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcRow := y * width * 4
		destRow := y * img.Stride
		for x := 0; x < width; x++ {
			b := src[srcRow+x*4+0]
			g := src[srcRow+x*4+1]
			r := src[srcRow+x*4+2]
			a := src[srcRow+x*4+3]

			img.Pix[destRow+x*4+0] = r
			img.Pix[destRow+x*4+1] = g
			img.Pix[destRow+x*4+2] = b
			img.Pix[destRow+x*4+3] = a
		}
	}
	return img
}
