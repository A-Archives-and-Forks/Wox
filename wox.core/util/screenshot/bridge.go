package screenshot

import (
	"image"
	"image/color"
)

type EventType string

const (
	EventTypeMouseDown      EventType = "mouse_down"
	EventTypeMouseMove      EventType = "mouse_move"
	EventTypeMouseUp        EventType = "mouse_up"
	EventTypeKeyDown        EventType = "key_down"
	EventTypeToolSelected   EventType = "tool_selected"
	EventTypePropertyChange EventType = "property_change"
	EventTypeToolbarAction  EventType = "toolbar_action"
	EventTypeWindowClosed   EventType = "window_closed"
)

type MouseButton string

const (
	MouseButtonLeft  MouseButton = "left"
	MouseButtonRight MouseButton = "right"
	MouseButtonNone  MouseButton = "none"
)

type ToolbarAction string

const (
	ToolbarActionUndo    ToolbarAction = "undo"
	ToolbarActionRedo    ToolbarAction = "redo"
	ToolbarActionConfirm ToolbarAction = "confirm"
	ToolbarActionCancel  ToolbarAction = "cancel"
)

type ShellEvent struct {
	Type          EventType
	Point         Point
	Button        MouseButton
	Key           string
	Tool          Tool
	Action        ToolbarAction
	PropertyName  string
	ColorValue    color.NRGBA
	FloatValue    float64
	StringValue   string
	ModifierShift bool
	ModifierAlt   bool
	ModifierCtrl  bool
	ModifierMeta  bool
}

type SelectionHandle string

const (
	SelectionHandleNone  SelectionHandle = "none"
	SelectionHandleMove  SelectionHandle = "move"
	SelectionHandleNorth SelectionHandle = "north"
	SelectionHandleSouth SelectionHandle = "south"
	SelectionHandleEast  SelectionHandle = "east"
	SelectionHandleWest  SelectionHandle = "west"
	SelectionHandleNE    SelectionHandle = "north_east"
	SelectionHandleNW    SelectionHandle = "north_west"
	SelectionHandleSE    SelectionHandle = "south_east"
	SelectionHandleSW    SelectionHandle = "south_west"
)

type CursorType string

const (
	CursorTypeCrosshair CursorType = "crosshair"
	CursorTypeMove      CursorType = "move"
	CursorTypeResizeNS  CursorType = "resize_ns"
	CursorTypeResizeEW  CursorType = "resize_ew"
	CursorTypeResizeNW  CursorType = "resize_nw"
	CursorTypeResizeNE  CursorType = "resize_ne"
	CursorTypeText      CursorType = "text"
	CursorTypeDefault   CursorType = "default"
)

type ToolState struct {
	ActiveTool   Tool
	Stroke       StrokeStyle
	Text         TextStyle
	CanUndo      bool
	CanRedo      bool
	AllowConfirm bool
}

type ViewModel struct {
	State            SessionState
	Displays         []Display
	VirtualBounds    Rect
	Selection        Rect
	ShowToolbar      bool
	ShowProperties   bool
	ActiveHandle     SelectionHandle
	Cursor           CursorType
	Document         Document
	Preview          *Annotation
	ToolState        ToolState
	DimensionLabel   string
	ToolbarAnchor    Point
	PropertiesAnchor Point
}

type EventHandler interface {
	HandleShellEvent(event ShellEvent) error
}

type Shell interface {
	Run(sessionID string, initial ViewModel, handler EventHandler) error
	Update(view ViewModel) error
	Close() error
}

type CaptureProvider interface {
	ListDisplays() ([]Display, error)
	CaptureDisplays(displayIDs []string) (*image.RGBA, error)
	CaptureRect(rect PixelRect) (*image.RGBA, error)
}
