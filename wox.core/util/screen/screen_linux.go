//go:build linux && cgo

package screen

import (
	"fmt"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <stdlib.h>

Display* openDisplay() {
    return XOpenDisplay(NULL);
}

void getScreenSize(Display* display, int* width, int* height) {
    Screen* screen = DefaultScreenOfDisplay(display);
    *width = WidthOfScreen(screen);
    *height = HeightOfScreen(screen);
}

void closeDisplay(Display* display) {
    XCloseDisplay(display);
}
*/
import "C"

func GetMouseScreenGtk() (Size, error) {
	err := gtk.InitCheck(nil)
	if err != nil {
		return Size{}, err
	}

	default_gdk_display, err := gdk.DisplayGetDefault()
	if err != nil {
		return Size{}, err
	}

	monitor, err := default_gdk_display.GetPrimaryMonitor()
	if err != nil {
		return Size{}, err
	}

	area := monitor.GetWorkarea()
	return Size{
		Width:  int(area.GetWidth()),
		Height: int(area.GetHeight()),
	}, nil
}

func GetMouseScreen() Size {
	// Give gtk a try, as it considers DPI and scaling of the screen
	size, err := GetMouseScreenGtk()
	if err == nil {
		return size
	}
	// Fallback to X11
	display := C.openDisplay()
	if display == nil {
		panic("Could not open X11 display")
	}
	defer C.closeDisplay(display)

	var width, height C.int
	C.getScreenSize(display, &width, &height)

	return Size{
		Width:  int(width),
		Height: int(height),
	}
}

func GetActiveScreen() Size {
	// For Linux, we'll use the mouse screen info
	// Note: Getting the truly active screen in Linux is complex and requires window manager integration
	return GetMouseScreen()
}

func listDisplays() ([]Display, error) {
	err := gtk.InitCheck(nil)
	if err != nil {
		return nil, err
	}

	display, err := gdk.DisplayGetDefault()
	if err != nil {
		return nil, err
	}

	count := display.GetNMonitors()
	displays := make([]Display, 0, count)
	for i := 0; i < count; i++ {
		monitor, monitorErr := display.GetMonitor(i)
		if monitorErr != nil {
			return nil, monitorErr
		}

		geometry := monitor.GetGeometry()
		workarea := monitor.GetWorkarea()
		scale := float64(monitor.GetScaleFactor())
		if scale <= 0 {
			scale = 1
		}

		displays = append(displays, Display{
			ID:   fmt.Sprintf("%d", i),
			Name: fmt.Sprintf("Display %d", i+1),
			Bounds: Rect{
				X:      int(geometry.GetX()),
				Y:      int(geometry.GetY()),
				Width:  int(geometry.GetWidth()),
				Height: int(geometry.GetHeight()),
			},
			WorkArea: Rect{
				X:      int(workarea.GetX()),
				Y:      int(workarea.GetY()),
				Width:  int(workarea.GetWidth()),
				Height: int(workarea.GetHeight()),
			},
			PixelBounds: Rect{
				X:      int(float64(geometry.GetX()) * scale),
				Y:      int(float64(geometry.GetY()) * scale),
				Width:  int(float64(geometry.GetWidth()) * scale),
				Height: int(float64(geometry.GetHeight()) * scale),
			},
			PixelWorkArea: Rect{
				X:      int(float64(workarea.GetX()) * scale),
				Y:      int(float64(workarea.GetY()) * scale),
				Width:  int(float64(workarea.GetWidth()) * scale),
				Height: int(float64(workarea.GetHeight()) * scale),
			},
			Scale:   scale,
			Primary: i == 0,
		})
	}

	return displays, nil
}
