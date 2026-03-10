package screenshot

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sync"

	"github.com/google/uuid"
)

type SessionResult struct {
	Image      image.Image
	FilePath   string
	Selection  Rect
	Document   Document
	DisplayIDs []string
}

type Manager interface {
	StartSession(ctx context.Context, opts StartOptions) (*SessionResult, error)
	CancelSession(sessionID string) error
	GetSession(sessionID string) (*Session, bool)
}

type Session struct {
	mu            sync.RWMutex
	ID            string
	State         SessionState
	Options       StartOptions
	Displays      []Display
	VirtualBounds Rect
	Selection     Rect
	Document      Document
	ToolState     ToolState
	ActiveHandle  SelectionHandle
	Cursor        CursorType
	dragging      bool
	dragOrigin    Point
	dragSelection Rect
	dragHandle    SelectionHandle
	shell         Shell
	onViewUpdated func(ViewModel)
}

type manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	capture  CaptureProvider
	renderer Renderer
	exporter Exporter
	newShell func() Shell
}

type sessionOutcome struct {
	result *SessionResult
	err    error
}

func NewManager() Manager {
	return newManagerWithDependencies(newCaptureProvider(), newShell, newRenderer(), newExporter())
}

func newManagerWithDependencies(capture CaptureProvider, shellFactory func() Shell, renderer Renderer, exporter Exporter) *manager {
	return &manager{
		sessions: make(map[string]*Session),
		capture:  capture,
		renderer: renderer,
		exporter: exporter,
		newShell: shellFactory,
	}
}

func (m *manager) StartSession(ctx context.Context, opts StartOptions) (*SessionResult, error) {
	if opts.InitialTool == "" {
		opts.InitialTool = ToolSelect
	}

	displays, err := m.capture.ListDisplays()
	if err != nil {
		return nil, err
	}

	if len(opts.DisplayIDs) > 0 {
		displays = filterDisplays(displays, opts.DisplayIDs)
	}
	if len(displays) == 0 {
		return nil, ErrNoDisplays
	}

	shell := m.newShell()
	if shell == nil {
		return nil, ErrNotImplemented
	}

	session := &Session{
		ID:            uuid.NewString(),
		State:         SessionStateSelecting,
		Options:       opts,
		Displays:      displays,
		VirtualBounds: GetVirtualBounds(displays),
		ToolState: ToolState{
			ActiveTool: opts.InitialTool,
			Stroke: StrokeStyle{
				Color: color.NRGBA{R: 0, G: 255, B: 160, A: 255},
				Width: 2,
			},
			Text: TextStyle{
				Color:    color.NRGBA{R: 255, G: 255, B: 255, A: 255},
				FontSize: 14,
			},
		},
		Cursor: CursorTypeCrosshair,
		shell:  shell,
	}
	session.ActiveHandle = SelectionHandleNone

	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()

	outcomeCh := make(chan sessionOutcome, 1)
	var finishOnce sync.Once
	finish := func(result *SessionResult, err error) {
		finishOnce.Do(func() {
			if closeErr := m.finalizeSession(session.ID, shell); err == nil && closeErr != nil {
				err = closeErr
			}
			outcomeCh <- sessionOutcome{
				result: result,
				err:    err,
			}
		})
	}

	session.SetViewUpdatedCallback(func(view ViewModel) {
		switch view.State {
		case SessionStateExporting:
			go func() {
				result, err := m.exportSession(session)
				finish(result, err)
			}()
		case SessionStateCancelled:
			go finish(nil, ErrSessionCancelled)
		case SessionStateClosed:
			go finish(nil, ErrSessionClosed)
		default:
			if err := shell.Update(view); err != nil && err != ErrSessionClosed {
				go finish(nil, err)
			}
		}
	})

	if err := shell.Run(session.ID, session.SnapshotViewModel(), session); err != nil {
		_ = m.finalizeSession(session.ID, shell)
		return nil, err
	}

	select {
	case outcome := <-outcomeCh:
		return outcome.result, outcome.err
	case <-ctx.Done():
		finish(nil, ctx.Err())
		outcome := <-outcomeCh
		return outcome.result, outcome.err
	}
}

func (m *manager) CancelSession(sessionID string) error {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return ErrSessionNotFound
	}

	session.mu.Lock()
	session.State = SessionStateCancelled
	session.mu.Unlock()

	return m.finalizeSession(sessionID, session.shell)
}

func (m *manager) GetSession(sessionID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	return session, ok
}

func filterDisplays(displays []Display, ids []string) []Display {
	if len(ids) == 0 {
		return displays
	}

	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}

	filtered := make([]Display, 0, len(displays))
	for _, display := range displays {
		if _, ok := allowed[display.ID]; ok {
			filtered = append(filtered, display)
		}
	}

	return filtered
}

func (m *manager) finalizeSession(sessionID string, shell Shell) error {
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	if shell == nil {
		return nil
	}

	if err := shell.Close(); err != nil && err != ErrSessionClosed {
		return err
	}

	return nil
}

func (m *manager) exportSession(session *Session) (*SessionResult, error) {
	view := session.SnapshotViewModel()
	if view.Selection.IsEmpty() {
		return nil, ErrEmptySelection
	}

	baseImage, err := m.captureSelection(view.Displays, view.Selection)
	if err != nil {
		return nil, err
	}

	rendered, err := m.renderer.Render(RenderInput{
		BaseImage: baseImage,
		Document:  view.Document,
		Selection: view.Selection,
	})
	if err != nil {
		return nil, err
	}

	exportResult, err := m.exporter.Export(ExportRequest{
		Targets:   exportTargetsFromOptions(session.Options),
		Image:     rendered,
		Selection: view.Selection,
		Document:  view.Document,
		TempDir:   session.Options.TempDir,
	})
	if err != nil {
		return nil, err
	}

	result := &SessionResult{
		Image:      rendered,
		Selection:  view.Selection,
		Document:   view.Document,
		DisplayIDs: displayIDsForSelection(view.Displays, view.Selection),
	}
	if exportResult != nil {
		result.FilePath = exportResult.FilePath
	}

	return result, nil
}

func (m *manager) captureSelection(displays []Display, selection Rect) (*image.RGBA, error) {
	selection = selection.Normalize()
	if selection.IsEmpty() {
		return nil, ErrEmptySelection
	}

	type capturePiece struct {
		displayID string
		src       PixelRect
		dest      PixelRect
	}

	pieces := make([]capturePiece, 0, len(displays))
	destRects := make([]PixelRect, 0, len(displays))
	for _, display := range displays {
		intersection, ok := intersectRect(selection, display.Bounds)
		if !ok {
			continue
		}

		srcRect := logicalRectToDisplayPixelRect(display, intersection)
		destRect := logicalRectToGlobalPixelRect(display, intersection)
		pieces = append(pieces, capturePiece{
			displayID: display.ID,
			src:       srcRect,
			dest:      destRect,
		})
		destRects = append(destRects, destRect)
	}

	if len(pieces) == 0 {
		return nil, fmt.Errorf("selection %+v does not intersect any display", selection)
	}

	union := unionPixelRect(destRects)
	canvas := image.NewRGBA(image.Rect(0, 0, union.Width, union.Height))
	for _, piece := range pieces {
		displayImage, err := m.capture.CaptureDisplays([]string{piece.displayID})
		if err != nil {
			return nil, err
		}

		sourcePoint := image.Point{X: piece.src.X, Y: piece.src.Y}
		destRect := image.Rect(
			piece.dest.X-union.X,
			piece.dest.Y-union.Y,
			piece.dest.X-union.X+piece.dest.Width,
			piece.dest.Y-union.Y+piece.dest.Height,
		)
		draw.Draw(canvas, destRect, displayImage, sourcePoint, draw.Src)
	}

	return canvas, nil
}

func exportTargetsFromOptions(opts StartOptions) []ExportTarget {
	targets := make([]ExportTarget, 0, 2)
	if opts.CopyToClipboard {
		targets = append(targets, ExportTargetClipboard)
	}
	if opts.SaveToFile {
		targets = append(targets, ExportTargetTempFile)
	}
	return targets
}

func displayIDsForSelection(displays []Display, selection Rect) []string {
	selection = selection.Normalize()
	ids := make([]string, 0, len(displays))
	for _, display := range displays {
		if _, ok := intersectRect(selection, display.Bounds); ok {
			ids = append(ids, display.ID)
		}
	}
	return ids
}

func (s *Session) SetViewUpdatedCallback(callback func(ViewModel)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onViewUpdated = callback
}

func (s *Session) SnapshotViewModel() ViewModel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buildViewModelLocked()
}

func (s *Session) HandleShellEvent(event ShellEvent) error {
	s.mu.Lock()

	switch event.Type {
	case EventTypeMouseDown:
		if err := s.handleMouseDownLocked(event); err != nil {
			s.mu.Unlock()
			return err
		}
	case EventTypeMouseMove:
		if err := s.handleMouseMoveLocked(event); err != nil {
			s.mu.Unlock()
			return err
		}
	case EventTypeMouseUp:
		if err := s.handleMouseUpLocked(event); err != nil {
			s.mu.Unlock()
			return err
		}
	case EventTypeToolSelected:
		s.ToolState.ActiveTool = event.Tool
		if !s.Selection.IsEmpty() && event.Tool != ToolSelect {
			s.State = SessionStateAnnotating
			s.ActiveHandle = SelectionHandleNone
			s.Cursor = CursorTypeCrosshair
		} else {
			s.updateHoverStateLocked(event.Point)
		}
	case EventTypePropertyChange:
		s.handlePropertyChangeLocked(event)
	case EventTypeToolbarAction:
		s.handleToolbarActionLocked(event)
	case EventTypeKeyDown:
		s.handleKeyDownLocked(event)
	case EventTypeWindowClosed:
		s.State = SessionStateClosed
	}

	callback := s.onViewUpdated
	view := s.buildViewModelLocked()
	s.mu.Unlock()
	if callback != nil {
		callback(view)
	}
	return nil
}

func (s *Session) handleMouseDownLocked(event ShellEvent) error {
	if event.Button != MouseButtonLeft {
		return nil
	}

	switch s.State {
	case SessionStateSelecting, SessionStateSelected, SessionStateAnnotating:
		if s.ToolState.ActiveTool == ToolSelect && !s.Selection.IsEmpty() {
			handle := hitTestSelectionHandle(s.Selection, event.Point)
			if handle != SelectionHandleNone {
				s.dragging = true
				s.dragOrigin = event.Point
				s.dragSelection = s.Selection.Normalize()
				s.dragHandle = handle
				s.ActiveHandle = handle
				s.Cursor = cursorForHandle(handle, true, s.ToolState.ActiveTool)
				return nil
			}
		}

		s.dragging = true
		s.dragOrigin = event.Point
		s.dragSelection = Rect{}
		s.dragHandle = SelectionHandleNone
		s.ActiveHandle = SelectionHandleNone
		s.Selection = Rect{
			X:      event.Point.X,
			Y:      event.Point.Y,
			Width:  0,
			Height: 0,
		}
		s.State = SessionStateSelecting
		s.Cursor = CursorTypeCrosshair
	}

	return nil
}

func (s *Session) handleMouseMoveLocked(event ShellEvent) error {
	if !s.dragging {
		s.updateHoverStateLocked(event.Point)
		return nil
	}

	if s.dragHandle == SelectionHandleNone {
		s.Selection = Rect{
			X:      s.dragOrigin.X,
			Y:      s.dragOrigin.Y,
			Width:  event.Point.X - s.dragOrigin.X,
			Height: event.Point.Y - s.dragOrigin.Y,
		}.Normalize()
	} else if s.dragHandle == SelectionHandleMove {
		s.Selection = moveSelection(s.dragSelection, Point{
			X: event.Point.X - s.dragOrigin.X,
			Y: event.Point.Y - s.dragOrigin.Y,
		}, s.VirtualBounds)
	} else {
		s.Selection = resizeSelection(s.dragSelection, s.dragHandle, event.Point, s.VirtualBounds)
	}

	s.ToolState.AllowConfirm = !s.Selection.IsEmpty()
	return nil
}

func (s *Session) handleMouseUpLocked(event ShellEvent) error {
	if event.Button != MouseButtonLeft || !s.dragging {
		return nil
	}

	s.dragging = false
	if s.dragHandle == SelectionHandleNone {
		s.Selection = Rect{
			X:      s.dragOrigin.X,
			Y:      s.dragOrigin.Y,
			Width:  event.Point.X - s.dragOrigin.X,
			Height: event.Point.Y - s.dragOrigin.Y,
		}.Normalize()
	} else if s.dragHandle == SelectionHandleMove {
		s.Selection = moveSelection(s.dragSelection, Point{
			X: event.Point.X - s.dragOrigin.X,
			Y: event.Point.Y - s.dragOrigin.Y,
		}, s.VirtualBounds)
	} else {
		s.Selection = resizeSelection(s.dragSelection, s.dragHandle, event.Point, s.VirtualBounds)
	}
	s.dragSelection = Rect{}
	s.dragHandle = SelectionHandleNone

	if s.Selection.IsEmpty() {
		s.State = SessionStateSelecting
		s.ToolState.AllowConfirm = false
		s.ActiveHandle = SelectionHandleNone
		s.Cursor = cursorForHandle(SelectionHandleNone, false, s.ToolState.ActiveTool)
		return nil
	}

	if s.ToolState.ActiveTool == ToolSelect {
		s.State = SessionStateSelected
	} else {
		s.State = SessionStateAnnotating
	}
	s.ToolState.AllowConfirm = true
	s.updateHoverStateLocked(event.Point)
	return nil
}

func (s *Session) handlePropertyChangeLocked(event ShellEvent) {
	switch event.PropertyName {
	case "stroke_width":
		if event.FloatValue > 0 {
			s.ToolState.Stroke.Width = event.FloatValue
		}
	case "stroke_color":
		s.ToolState.Stroke.Color = event.ColorValue
	case "text_color":
		s.ToolState.Text.Color = event.ColorValue
	case "font_size":
		if event.FloatValue > 0 {
			s.ToolState.Text.FontSize = event.FloatValue
		}
	case "font_name":
		s.ToolState.Text.FontName = event.StringValue
	}
}

func (s *Session) handleToolbarActionLocked(event ShellEvent) {
	switch event.Action {
	case ToolbarActionCancel:
		s.State = SessionStateCancelled
	case ToolbarActionConfirm:
		if !s.Selection.IsEmpty() {
			s.State = SessionStateExporting
		}
	case ToolbarActionUndo:
		s.ToolState.CanUndo = false
		s.ToolState.CanRedo = true
	case ToolbarActionRedo:
		s.ToolState.CanRedo = false
	}
}

func (s *Session) handleKeyDownLocked(event ShellEvent) {
	switch event.Key {
	case "Escape", "Esc":
		s.State = SessionStateCancelled
	case "Enter", "Return":
		if !s.Selection.IsEmpty() {
			s.State = SessionStateExporting
		}
	}
}

func (s *Session) updateHoverStateLocked(point Point) {
	if s.dragging {
		return
	}

	hasSelection := !s.Selection.IsEmpty()
	if s.ToolState.ActiveTool != ToolSelect || !hasSelection {
		s.ActiveHandle = SelectionHandleNone
		s.Cursor = cursorForHandle(SelectionHandleNone, hasSelection, s.ToolState.ActiveTool)
		return
	}

	s.ActiveHandle = hitTestSelectionHandle(s.Selection, point)
	s.Cursor = cursorForHandle(s.ActiveHandle, true, s.ToolState.ActiveTool)
}

func (s *Session) buildViewModelLocked() ViewModel {
	selection := s.Selection.Normalize()
	showToolbar := !selection.IsEmpty() && s.State != SessionStateCancelled && s.State != SessionStateClosed
	showProperties := showToolbar && s.ToolState.ActiveTool != ToolSelect
	vm := ViewModel{
		State:          s.State,
		Displays:       append([]Display(nil), s.Displays...),
		VirtualBounds:  s.VirtualBounds,
		Selection:      selection,
		ShowToolbar:    showToolbar,
		ShowProperties: showProperties,
		ActiveHandle:   s.ActiveHandle,
		Cursor:         s.Cursor,
		Document:       s.Document,
		ToolState:      s.ToolState,
		DimensionLabel: selection.Label(),
	}

	if !selection.IsEmpty() {
		vm.ToolbarAnchor = Point{
			X: selection.X + selection.Width/2,
			Y: selection.Y + selection.Height + 12,
		}
		vm.PropertiesAnchor = Point{
			X: selection.X + selection.Width + 12,
			Y: selection.Y,
		}
	}

	return vm
}
