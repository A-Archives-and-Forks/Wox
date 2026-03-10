package screenshot

import "image/color"

type CaptureMode string

const (
	CaptureModeRegion  CaptureMode = "region"
	CaptureModeDisplay CaptureMode = "display"
	CaptureModeAll     CaptureMode = "all"
)

type SessionState string

const (
	SessionStateIdle       SessionState = "idle"
	SessionStatePreparing  SessionState = "preparing"
	SessionStateSelecting  SessionState = "selecting"
	SessionStateSelected   SessionState = "selected"
	SessionStateAnnotating SessionState = "annotating"
	SessionStateExporting  SessionState = "exporting"
	SessionStateCancelled  SessionState = "cancelled"
	SessionStateClosed     SessionState = "closed"
)

type Tool string

const (
	ToolSelect Tool = "select"
	ToolRect   Tool = "rect"
	ToolArrow  Tool = "arrow"
	ToolPen    Tool = "pen"
	ToolText   Tool = "text"
)

type Point struct {
	X float64
	Y float64
}

type Rect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type PixelRect struct {
	X      int
	Y      int
	Width  int
	Height int
}

type Display struct {
	ID          string
	Name        string
	Bounds      Rect
	PixelBounds PixelRect
	Scale       float64
	Primary     bool
}

type AnnotationType string

const (
	AnnotationTypeRect  AnnotationType = "rect"
	AnnotationTypeArrow AnnotationType = "arrow"
	AnnotationTypePen   AnnotationType = "pen"
	AnnotationTypeText  AnnotationType = "text"
)

type StrokeStyle struct {
	Color color.NRGBA
	Width float64
}

type TextStyle struct {
	Color    color.NRGBA
	FontSize float64
	FontName string
}

type Annotation struct {
	ID         string
	Type       AnnotationType
	Bounds     Rect
	Points     []Point
	Text       string
	Stroke     StrokeStyle
	TextStyle  TextStyle
	ZIndex     int
	IsSelected bool
}

type Document struct {
	Annotations []Annotation
}

type ExportTarget string

const (
	ExportTargetClipboard ExportTarget = "clipboard"
	ExportTargetTempFile  ExportTarget = "temp_file"
)

type StartOptions struct {
	Mode            CaptureMode
	DisplayIDs      []string
	CopyToClipboard bool
	SaveToFile      bool
	TempDir         string
	InitialTool     Tool
}
