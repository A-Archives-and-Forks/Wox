package screenshot

import (
	"context"
	"image"
	"image/color"
	"testing"
)

func TestSessionSelectionFlow(t *testing.T) {
	session := &Session{
		State: SessionStateSelecting,
		ToolState: ToolState{
			ActiveTool: ToolSelect,
		},
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:   EventTypeMouseDown,
		Button: MouseButtonLeft,
		Point:  Point{X: 10, Y: 20},
	}); err != nil {
		t.Fatalf("mouse down failed: %v", err)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:  EventTypeMouseMove,
		Point: Point{X: 110, Y: 70},
	}); err != nil {
		t.Fatalf("mouse move failed: %v", err)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:   EventTypeMouseUp,
		Button: MouseButtonLeft,
		Point:  Point{X: 110, Y: 70},
	}); err != nil {
		t.Fatalf("mouse up failed: %v", err)
	}

	if session.State != SessionStateSelected {
		t.Fatalf("expected selected state, got %s", session.State)
	}

	if session.Selection.Width != 100 || session.Selection.Height != 50 {
		t.Fatalf("unexpected selection: %+v", session.Selection)
	}

	vm := session.SnapshotViewModel()
	if vm.DimensionLabel != "100 x 50" {
		t.Fatalf("unexpected dimension label: %s", vm.DimensionLabel)
	}
}

func TestSessionToolbarAndPropertyFlow(t *testing.T) {
	session := &Session{
		State: SessionStateSelected,
		Selection: Rect{
			X:      10,
			Y:      20,
			Width:  100,
			Height: 50,
		},
		ToolState: ToolState{
			ActiveTool: ToolSelect,
		},
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type: EventTypeToolSelected,
		Tool: ToolPen,
	}); err != nil {
		t.Fatalf("tool select failed: %v", err)
	}

	if session.State != SessionStateAnnotating {
		t.Fatalf("expected annotating state, got %s", session.State)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:         EventTypePropertyChange,
		PropertyName: "stroke_width",
		FloatValue:   6,
	}); err != nil {
		t.Fatalf("property change failed: %v", err)
	}

	if session.ToolState.Stroke.Width != 6 {
		t.Fatalf("unexpected stroke width: %f", session.ToolState.Stroke.Width)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:   EventTypeToolbarAction,
		Action: ToolbarActionConfirm,
	}); err != nil {
		t.Fatalf("toolbar action failed: %v", err)
	}

	if session.State != SessionStateExporting {
		t.Fatalf("expected exporting state, got %s", session.State)
	}
}

func TestSessionMoveSelectionFlow(t *testing.T) {
	session := &Session{
		State: SessionStateSelected,
		Selection: Rect{
			X:      20,
			Y:      30,
			Width:  100,
			Height: 60,
		},
		VirtualBounds: Rect{
			X:      0,
			Y:      0,
			Width:  400,
			Height: 300,
		},
		ToolState: ToolState{
			ActiveTool: ToolSelect,
		},
		Cursor: CursorTypeDefault,
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:  EventTypeMouseMove,
		Point: Point{X: 50, Y: 50},
	}); err != nil {
		t.Fatalf("hover failed: %v", err)
	}

	if session.ActiveHandle != SelectionHandleMove {
		t.Fatalf("expected move handle, got %s", session.ActiveHandle)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:   EventTypeMouseDown,
		Button: MouseButtonLeft,
		Point:  Point{X: 50, Y: 50},
	}); err != nil {
		t.Fatalf("mouse down failed: %v", err)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:  EventTypeMouseMove,
		Point: Point{X: 80, Y: 85},
	}); err != nil {
		t.Fatalf("drag move failed: %v", err)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:   EventTypeMouseUp,
		Button: MouseButtonLeft,
		Point:  Point{X: 80, Y: 85},
	}); err != nil {
		t.Fatalf("mouse up failed: %v", err)
	}

	if session.Selection.X != 50 || session.Selection.Y != 65 {
		t.Fatalf("unexpected moved selection: %+v", session.Selection)
	}

	if session.State != SessionStateSelected {
		t.Fatalf("expected selected state, got %s", session.State)
	}
}

func TestSessionResizeSelectionFlow(t *testing.T) {
	session := &Session{
		State: SessionStateSelected,
		Selection: Rect{
			X:      20,
			Y:      30,
			Width:  100,
			Height: 60,
		},
		VirtualBounds: Rect{
			X:      0,
			Y:      0,
			Width:  400,
			Height: 300,
		},
		ToolState: ToolState{
			ActiveTool: ToolSelect,
		},
		Cursor: CursorTypeDefault,
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:  EventTypeMouseMove,
		Point: Point{X: 120, Y: 90},
	}); err != nil {
		t.Fatalf("hover failed: %v", err)
	}

	if session.ActiveHandle != SelectionHandleSE {
		t.Fatalf("expected south-east handle, got %s", session.ActiveHandle)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:   EventTypeMouseDown,
		Button: MouseButtonLeft,
		Point:  Point{X: 120, Y: 90},
	}); err != nil {
		t.Fatalf("mouse down failed: %v", err)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:  EventTypeMouseMove,
		Point: Point{X: 160, Y: 140},
	}); err != nil {
		t.Fatalf("drag move failed: %v", err)
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type:   EventTypeMouseUp,
		Button: MouseButtonLeft,
		Point:  Point{X: 160, Y: 140},
	}); err != nil {
		t.Fatalf("mouse up failed: %v", err)
	}

	if session.Selection.Width != 140 || session.Selection.Height != 110 {
		t.Fatalf("unexpected resized selection: %+v", session.Selection)
	}
}

func TestSessionViewModelChromeVisibility(t *testing.T) {
	session := &Session{
		State: SessionStateSelected,
		Selection: Rect{
			X:      20,
			Y:      30,
			Width:  100,
			Height: 60,
		},
		ToolState: ToolState{
			ActiveTool:   ToolPen,
			AllowConfirm: true,
		},
	}

	vm := session.SnapshotViewModel()
	if !vm.ShowToolbar {
		t.Fatalf("expected toolbar to be visible")
	}
	if !vm.ShowProperties {
		t.Fatalf("expected properties to be visible")
	}

	if err := session.HandleShellEvent(ShellEvent{
		Type: EventTypeToolSelected,
		Tool: ToolSelect,
	}); err != nil {
		t.Fatalf("tool select failed: %v", err)
	}

	vm = session.SnapshotViewModel()
	if !vm.ShowToolbar {
		t.Fatalf("expected toolbar to remain visible")
	}
	if vm.ShowProperties {
		t.Fatalf("expected properties to be hidden for select tool")
	}
}

type stubShell struct {
	runFn        func(sessionID string, initial ViewModel, handler EventHandler) error
	updatedViews []ViewModel
	closeCount   int
}

func (s *stubShell) Run(sessionID string, initial ViewModel, handler EventHandler) error {
	if s.runFn != nil {
		return s.runFn(sessionID, initial, handler)
	}
	return nil
}

func (s *stubShell) Update(view ViewModel) error {
	s.updatedViews = append(s.updatedViews, view)
	return nil
}

func (s *stubShell) Close() error {
	s.closeCount++
	return nil
}

type stubCaptureProvider struct {
	displays []Display
	images   map[string]*image.RGBA
}

func (p *stubCaptureProvider) ListDisplays() ([]Display, error) {
	return p.displays, nil
}

func (p *stubCaptureProvider) CaptureDisplays(displayIDs []string) (*image.RGBA, error) {
	if len(displayIDs) != 1 {
		return nil, ErrNotImplemented
	}

	return p.images[displayIDs[0]], nil
}

func (p *stubCaptureProvider) CaptureRect(rect PixelRect) (*image.RGBA, error) {
	_ = rect
	return nil, ErrNotImplemented
}

type stubExporter struct {
	lastRequest ExportRequest
	filePath    string
}

func (e *stubExporter) Export(req ExportRequest) (*ExportResult, error) {
	e.lastRequest = req
	return &ExportResult{FilePath: e.filePath}, nil
}

func TestManagerStartSessionConfirmFlow(t *testing.T) {
	display := Display{
		ID: "display-1",
		Bounds: Rect{
			X:      0,
			Y:      0,
			Width:  200,
			Height: 120,
		},
		PixelBounds: PixelRect{
			X:      0,
			Y:      0,
			Width:  200,
			Height: 120,
		},
		Scale:   1,
		Primary: true,
	}

	base := image.NewRGBA(image.Rect(0, 0, 200, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 200; x++ {
			base.Set(x, y, color.NRGBA{R: 40, G: 80, B: 120, A: 255})
		}
	}

	shell := &stubShell{}
	shell.runFn = func(sessionID string, initial ViewModel, handler EventHandler) error {
		_ = sessionID
		_ = initial

		if err := handler.HandleShellEvent(ShellEvent{
			Type:   EventTypeMouseDown,
			Button: MouseButtonLeft,
			Point:  Point{X: 10, Y: 15},
		}); err != nil {
			return err
		}
		if err := handler.HandleShellEvent(ShellEvent{
			Type:  EventTypeMouseMove,
			Point: Point{X: 110, Y: 55},
		}); err != nil {
			return err
		}
		if err := handler.HandleShellEvent(ShellEvent{
			Type:   EventTypeMouseUp,
			Button: MouseButtonLeft,
			Point:  Point{X: 110, Y: 55},
		}); err != nil {
			return err
		}
		return handler.HandleShellEvent(ShellEvent{
			Type:   EventTypeToolbarAction,
			Action: ToolbarActionConfirm,
		})
	}

	exporter := &stubExporter{filePath: "/tmp/test-screenshot.png"}
	manager := newManagerWithDependencies(
		&stubCaptureProvider{
			displays: []Display{display},
			images: map[string]*image.RGBA{
				display.ID: base,
			},
		},
		func() Shell { return shell },
		newRenderer(),
		exporter,
	)

	result, err := manager.StartSession(context.Background(), StartOptions{
		Mode:       CaptureModeRegion,
		SaveToFile: true,
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if result == nil {
		t.Fatalf("expected session result")
	}

	if result.Selection.Width != 100 || result.Selection.Height != 40 {
		t.Fatalf("unexpected selection: %+v", result.Selection)
	}

	if result.FilePath != "/tmp/test-screenshot.png" {
		t.Fatalf("unexpected file path: %s", result.FilePath)
	}

	if exporter.lastRequest.Image.Bounds().Dx() != 100 || exporter.lastRequest.Image.Bounds().Dy() != 40 {
		t.Fatalf("unexpected exported image size: %v", exporter.lastRequest.Image.Bounds())
	}

	if shell.closeCount != 1 {
		t.Fatalf("expected shell to close once, got %d", shell.closeCount)
	}
}

func TestManagerStartSessionCancelFlow(t *testing.T) {
	shell := &stubShell{}
	shell.runFn = func(sessionID string, initial ViewModel, handler EventHandler) error {
		_ = sessionID
		_ = initial
		return handler.HandleShellEvent(ShellEvent{
			Type: EventTypeKeyDown,
			Key:  "Escape",
		})
	}

	manager := newManagerWithDependencies(
		&stubCaptureProvider{
			displays: []Display{
				{
					ID: "display-1",
					Bounds: Rect{
						X:      0,
						Y:      0,
						Width:  100,
						Height: 100,
					},
					PixelBounds: PixelRect{
						X:      0,
						Y:      0,
						Width:  100,
						Height: 100,
					},
					Scale: 1,
				},
			},
			images: map[string]*image.RGBA{},
		},
		func() Shell { return shell },
		newRenderer(),
		&stubExporter{},
	)

	result, err := manager.StartSession(context.Background(), StartOptions{
		Mode: CaptureModeRegion,
	})
	if err != ErrSessionCancelled {
		t.Fatalf("expected ErrSessionCancelled, got %v", err)
	}

	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}

	if shell.closeCount != 1 {
		t.Fatalf("expected shell to close once, got %d", shell.closeCount)
	}
}
