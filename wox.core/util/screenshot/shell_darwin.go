//go:build darwin

package screenshot

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Cocoa

#include <stdlib.h>

typedef struct {
    double selectionX;
    double selectionY;
    double selectionWidth;
    double selectionHeight;
    double toolbarAnchorX;
    double toolbarAnchorY;
    double propertiesAnchorX;
    double propertiesAnchorY;
    double strokeWidth;
    double fontSize;
    int state;
    int showToolbar;
    int showProperties;
    int activeHandle;
    int cursor;
    int activeTool;
    int strokeColorR;
    int strokeColorG;
    int strokeColorB;
    int strokeColorA;
    int textColorR;
    int textColorG;
    int textColorB;
    int textColorA;
    int canUndo;
    int canRedo;
    int allowConfirm;
} ScreenshotShellViewModel;

void CreateScreenshotShell(const char* sessionID);
void UpdateScreenshotShell(const char* sessionID, ScreenshotShellViewModel viewModel);
void CloseScreenshotShell(const char* sessionID);
*/
import "C"
import (
	"image/color"
	"sync"
	"unsafe"

	"golang.design/x/hotkey/mainthread"
)

type nativeShell struct {
	sessionID string
}

type nativeCaptureProvider struct{}

var shellHandlers sync.Map

func newShell() Shell {
	return &nativeShell{}
}

func newCaptureProvider() CaptureProvider {
	return &nativeCaptureProvider{}
}

func (s *nativeShell) Run(sessionID string, initial ViewModel, handler EventHandler) error {
	s.sessionID = sessionID
	shellHandlers.Store(sessionID, handler)

	mainthread.Call(func() {
		cSessionID := C.CString(sessionID)
		defer C.free(unsafe.Pointer(cSessionID))
		C.CreateScreenshotShell(cSessionID)
		C.UpdateScreenshotShell(cSessionID, toDarwinViewModel(initial))
	})

	return nil
}

func (s *nativeShell) Update(view ViewModel) error {
	if s.sessionID == "" {
		return ErrSessionClosed
	}

	mainthread.Call(func() {
		cSessionID := C.CString(s.sessionID)
		defer C.free(unsafe.Pointer(cSessionID))
		C.UpdateScreenshotShell(cSessionID, toDarwinViewModel(view))
	})

	return nil
}

func (s *nativeShell) Close() error {
	if s.sessionID == "" {
		return nil
	}

	sessionID := s.sessionID
	s.sessionID = ""
	shellHandlers.Delete(sessionID)

	mainthread.Call(func() {
		cSessionID := C.CString(sessionID)
		defer C.free(unsafe.Pointer(cSessionID))
		C.CloseScreenshotShell(cSessionID)
	})

	return nil
}

func toDarwinViewModel(view ViewModel) C.ScreenshotShellViewModel {
	selection := view.Selection.Normalize()
	return C.ScreenshotShellViewModel{
		selectionX:        C.double(selection.X),
		selectionY:        C.double(selection.Y),
		selectionWidth:    C.double(selection.Width),
		selectionHeight:   C.double(selection.Height),
		toolbarAnchorX:    C.double(view.ToolbarAnchor.X),
		toolbarAnchorY:    C.double(view.ToolbarAnchor.Y),
		propertiesAnchorX: C.double(view.PropertiesAnchor.X),
		propertiesAnchorY: C.double(view.PropertiesAnchor.Y),
		strokeWidth:       C.double(view.ToolState.Stroke.Width),
		fontSize:          C.double(view.ToolState.Text.FontSize),
		state:             C.int(sessionStateCode(view.State)),
		showToolbar:       darwinBool(view.ShowToolbar),
		showProperties:    darwinBool(view.ShowProperties),
		activeHandle:      C.int(selectionHandleCode(view.ActiveHandle)),
		cursor:            C.int(cursorCode(view.Cursor)),
		activeTool:        C.int(toolCode(view.ToolState.ActiveTool)),
		strokeColorR:      C.int(view.ToolState.Stroke.Color.R),
		strokeColorG:      C.int(view.ToolState.Stroke.Color.G),
		strokeColorB:      C.int(view.ToolState.Stroke.Color.B),
		strokeColorA:      C.int(view.ToolState.Stroke.Color.A),
		textColorR:        C.int(view.ToolState.Text.Color.R),
		textColorG:        C.int(view.ToolState.Text.Color.G),
		textColorB:        C.int(view.ToolState.Text.Color.B),
		textColorA:        C.int(view.ToolState.Text.Color.A),
		canUndo:           darwinBool(view.ToolState.CanUndo),
		canRedo:           darwinBool(view.ToolState.CanRedo),
		allowConfirm:      darwinBool(view.ToolState.AllowConfirm),
	}
}

func darwinBool(value bool) C.int {
	if value {
		return 1
	}
	return 0
}

func sessionStateCode(state SessionState) int {
	switch state {
	case SessionStateSelecting:
		return 1
	case SessionStateSelected:
		return 2
	case SessionStateAnnotating:
		return 3
	case SessionStateExporting:
		return 4
	case SessionStateCancelled:
		return 5
	case SessionStateClosed:
		return 6
	default:
		return 0
	}
}

func selectionHandleCode(handle SelectionHandle) int {
	switch handle {
	case SelectionHandleMove:
		return 1
	case SelectionHandleNorth:
		return 2
	case SelectionHandleSouth:
		return 3
	case SelectionHandleEast:
		return 4
	case SelectionHandleWest:
		return 5
	case SelectionHandleNE:
		return 6
	case SelectionHandleNW:
		return 7
	case SelectionHandleSE:
		return 8
	case SelectionHandleSW:
		return 9
	default:
		return 0
	}
}

func cursorCode(cursor CursorType) int {
	switch cursor {
	case CursorTypeCrosshair:
		return 1
	case CursorTypeMove:
		return 2
	case CursorTypeResizeNS:
		return 3
	case CursorTypeResizeEW:
		return 4
	case CursorTypeResizeNW:
		return 5
	case CursorTypeResizeNE:
		return 6
	case CursorTypeText:
		return 7
	default:
		return 0
	}
}

func toolCode(tool Tool) int {
	switch tool {
	case ToolSelect:
		return 1
	case ToolRect:
		return 2
	case ToolArrow:
		return 3
	case ToolPen:
		return 4
	case ToolText:
		return 5
	default:
		return 0
	}
}

func toolFromCode(code C.int) Tool {
	switch int(code) {
	case 1:
		return ToolSelect
	case 2:
		return ToolRect
	case 3:
		return ToolArrow
	case 4:
		return ToolPen
	case 5:
		return ToolText
	default:
		return ToolSelect
	}
}

func toolbarActionFromCode(code C.int) ToolbarAction {
	switch int(code) {
	case 1:
		return ToolbarActionUndo
	case 2:
		return ToolbarActionRedo
	case 3:
		return ToolbarActionCancel
	case 4:
		return ToolbarActionConfirm
	default:
		return ""
	}
}

func dispatchDarwinShellEvent(sessionID string, event ShellEvent) {
	handlerAny, ok := shellHandlers.Load(sessionID)
	if !ok {
		return
	}

	handler, ok := handlerAny.(EventHandler)
	if !ok || handler == nil {
		return
	}

	_ = handler.HandleShellEvent(event)
}

//export screenshotShellMouseDownCGO
func screenshotShellMouseDownCGO(cSessionID *C.char, x C.double, y C.double, button C.int) {
	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type:   EventTypeMouseDown,
		Button: darwinMouseButton(button),
		Point: Point{
			X: float64(x),
			Y: float64(y),
		},
	})
}

//export screenshotShellMouseMoveCGO
func screenshotShellMouseMoveCGO(cSessionID *C.char, x C.double, y C.double) {
	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type: EventTypeMouseMove,
		Point: Point{
			X: float64(x),
			Y: float64(y),
		},
	})
}

//export screenshotShellMouseUpCGO
func screenshotShellMouseUpCGO(cSessionID *C.char, x C.double, y C.double, button C.int) {
	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type:   EventTypeMouseUp,
		Button: darwinMouseButton(button),
		Point: Point{
			X: float64(x),
			Y: float64(y),
		},
	})
}

//export screenshotShellKeyDownCGO
func screenshotShellKeyDownCGO(cSessionID *C.char, cKey *C.char) {
	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type: EventTypeKeyDown,
		Key:  C.GoString(cKey),
	})
}

//export screenshotShellToolSelectedCGO
func screenshotShellToolSelectedCGO(cSessionID *C.char, tool C.int) {
	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type: EventTypeToolSelected,
		Tool: toolFromCode(tool),
	})
}

//export screenshotShellToolbarActionCGO
func screenshotShellToolbarActionCGO(cSessionID *C.char, action C.int) {
	toolbarAction := toolbarActionFromCode(action)
	if toolbarAction == "" {
		return
	}

	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type:   EventTypeToolbarAction,
		Action: toolbarAction,
	})
}

//export screenshotShellPropertyFloatChangedCGO
func screenshotShellPropertyFloatChangedCGO(cSessionID *C.char, cName *C.char, value C.double) {
	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type:         EventTypePropertyChange,
		PropertyName: C.GoString(cName),
		FloatValue:   float64(value),
	})
}

//export screenshotShellPropertyColorChangedCGO
func screenshotShellPropertyColorChangedCGO(cSessionID *C.char, cName *C.char, r C.int, g C.int, b C.int, a C.int) {
	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type:         EventTypePropertyChange,
		PropertyName: C.GoString(cName),
		ColorValue: color.NRGBA{
			R: uint8(r),
			G: uint8(g),
			B: uint8(b),
			A: uint8(a),
		},
	})
}

//export screenshotShellClosedCGO
func screenshotShellClosedCGO(cSessionID *C.char) {
	dispatchDarwinShellEvent(C.GoString(cSessionID), ShellEvent{
		Type: EventTypeWindowClosed,
	})
}

func darwinMouseButton(button C.int) MouseButton {
	switch int(button) {
	case 1:
		return MouseButtonLeft
	case 2:
		return MouseButtonRight
	default:
		return MouseButtonNone
	}
}
