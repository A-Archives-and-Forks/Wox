package screenshot

import (
	"fmt"
	"math"
)

const (
	selectionHandleHitRadius = 8.0
	selectionMinSize         = 4.0
)

func (r Rect) IsEmpty() bool {
	return r.Width <= 0 || r.Height <= 0
}

func (r Rect) Normalize() Rect {
	normalized := r
	if normalized.Width < 0 {
		normalized.X += normalized.Width
		normalized.Width = -normalized.Width
	}
	if normalized.Height < 0 {
		normalized.Y += normalized.Height
		normalized.Height = -normalized.Height
	}
	return normalized
}

func (r Rect) Label() string {
	normalized := r.Normalize()
	return fmt.Sprintf("%.0f x %.0f", normalized.Width, normalized.Height)
}

func (r Rect) ContainsPoint(point Point) bool {
	normalized := r.Normalize()
	return point.X >= normalized.X && point.X <= normalized.X+normalized.Width &&
		point.Y >= normalized.Y && point.Y <= normalized.Y+normalized.Height
}

func (r Rect) ClampWithin(bounds Rect) Rect {
	normalized := r.Normalize()
	if bounds.IsEmpty() {
		return normalized
	}

	if normalized.Width > bounds.Width {
		normalized.Width = bounds.Width
	}
	if normalized.Height > bounds.Height {
		normalized.Height = bounds.Height
	}

	minX := bounds.X
	maxX := bounds.X + bounds.Width - normalized.Width
	minY := bounds.Y
	maxY := bounds.Y + bounds.Height - normalized.Height

	normalized.X = clampFloat(normalized.X, minX, maxX)
	normalized.Y = clampFloat(normalized.Y, minY, maxY)
	return normalized
}

func selectionHandlePoint(rect Rect, handle SelectionHandle) Point {
	normalized := rect.Normalize()
	centerX := normalized.X + normalized.Width/2
	centerY := normalized.Y + normalized.Height/2
	right := normalized.X + normalized.Width
	bottom := normalized.Y + normalized.Height

	switch handle {
	case SelectionHandleNorth:
		return Point{X: centerX, Y: normalized.Y}
	case SelectionHandleSouth:
		return Point{X: centerX, Y: bottom}
	case SelectionHandleEast:
		return Point{X: right, Y: centerY}
	case SelectionHandleWest:
		return Point{X: normalized.X, Y: centerY}
	case SelectionHandleNE:
		return Point{X: right, Y: normalized.Y}
	case SelectionHandleNW:
		return Point{X: normalized.X, Y: normalized.Y}
	case SelectionHandleSE:
		return Point{X: right, Y: bottom}
	case SelectionHandleSW:
		return Point{X: normalized.X, Y: bottom}
	default:
		return Point{}
	}
}

func hitTestSelectionHandle(rect Rect, point Point) SelectionHandle {
	normalized := rect.Normalize()
	if normalized.IsEmpty() {
		return SelectionHandleNone
	}

	orderedHandles := []SelectionHandle{
		SelectionHandleNW,
		SelectionHandleNE,
		SelectionHandleSW,
		SelectionHandleSE,
		SelectionHandleNorth,
		SelectionHandleSouth,
		SelectionHandleWest,
		SelectionHandleEast,
	}

	for _, handle := range orderedHandles {
		handlePoint := selectionHandlePoint(normalized, handle)
		if distance(point, handlePoint) <= selectionHandleHitRadius {
			return handle
		}
	}

	if normalized.ContainsPoint(point) {
		return SelectionHandleMove
	}

	return SelectionHandleNone
}

func cursorForHandle(handle SelectionHandle, hasSelection bool, activeTool Tool) CursorType {
	if activeTool != ToolSelect {
		return CursorTypeCrosshair
	}
	if !hasSelection {
		return CursorTypeCrosshair
	}

	switch handle {
	case SelectionHandleMove:
		return CursorTypeMove
	case SelectionHandleNorth, SelectionHandleSouth:
		return CursorTypeResizeNS
	case SelectionHandleEast, SelectionHandleWest:
		return CursorTypeResizeEW
	case SelectionHandleNW, SelectionHandleSE:
		return CursorTypeResizeNW
	case SelectionHandleNE, SelectionHandleSW:
		return CursorTypeResizeNE
	default:
		return CursorTypeCrosshair
	}
}

func moveSelection(rect Rect, delta Point, bounds Rect) Rect {
	normalized := rect.Normalize()
	normalized.X += delta.X
	normalized.Y += delta.Y
	return normalized.ClampWithin(bounds)
}

func resizeSelection(rect Rect, handle SelectionHandle, point Point, bounds Rect) Rect {
	normalized := rect.Normalize()

	left := normalized.X
	top := normalized.Y
	right := normalized.X + normalized.Width
	bottom := normalized.Y + normalized.Height

	minLeft := bounds.X
	maxRight := bounds.X + bounds.Width
	minTop := bounds.Y
	maxBottom := bounds.Y + bounds.Height

	switch handle {
	case SelectionHandleNorth, SelectionHandleNE, SelectionHandleNW:
		top = clampFloat(point.Y, minTop, bottom-selectionMinSize)
	case SelectionHandleSouth, SelectionHandleSE, SelectionHandleSW:
		bottom = clampFloat(point.Y, top+selectionMinSize, maxBottom)
	}

	switch handle {
	case SelectionHandleWest, SelectionHandleNW, SelectionHandleSW:
		left = clampFloat(point.X, minLeft, right-selectionMinSize)
	case SelectionHandleEast, SelectionHandleNE, SelectionHandleSE:
		right = clampFloat(point.X, left+selectionMinSize, maxRight)
	}

	return Rect{
		X:      left,
		Y:      top,
		Width:  right - left,
		Height: bottom - top,
	}.Normalize()
}

func GetVirtualBounds(displays []Display) Rect {
	if len(displays) == 0 {
		return Rect{}
	}

	minX := displays[0].Bounds.X
	minY := displays[0].Bounds.Y
	maxRight := displays[0].Bounds.X + displays[0].Bounds.Width
	maxBottom := displays[0].Bounds.Y + displays[0].Bounds.Height

	for i := 1; i < len(displays); i++ {
		bounds := displays[i].Bounds
		if bounds.X < minX {
			minX = bounds.X
		}
		if bounds.Y < minY {
			minY = bounds.Y
		}
		if bounds.X+bounds.Width > maxRight {
			maxRight = bounds.X + bounds.Width
		}
		if bounds.Y+bounds.Height > maxBottom {
			maxBottom = bounds.Y + bounds.Height
		}
	}

	return Rect{
		X:      minX,
		Y:      minY,
		Width:  maxRight - minX,
		Height: maxBottom - minY,
	}
}

func intersectRect(a Rect, b Rect) (Rect, bool) {
	left := math.Max(a.X, b.X)
	top := math.Max(a.Y, b.Y)
	right := math.Min(a.X+a.Width, b.X+b.Width)
	bottom := math.Min(a.Y+a.Height, b.Y+b.Height)
	if right <= left || bottom <= top {
		return Rect{}, false
	}

	return Rect{
		X:      left,
		Y:      top,
		Width:  right - left,
		Height: bottom - top,
	}, true
}

func logicalRectToDisplayPixelRect(display Display, rect Rect) PixelRect {
	scaleX := display.Scale
	scaleY := display.Scale
	if display.Bounds.Width > 0 {
		scaleX = float64(display.PixelBounds.Width) / display.Bounds.Width
	}
	if display.Bounds.Height > 0 {
		scaleY = float64(display.PixelBounds.Height) / display.Bounds.Height
	}

	left := int(math.Floor((rect.X - display.Bounds.X) * scaleX))
	top := int(math.Floor((rect.Y - display.Bounds.Y) * scaleY))
	right := int(math.Ceil((rect.X + rect.Width - display.Bounds.X) * scaleX))
	bottom := int(math.Ceil((rect.Y + rect.Height - display.Bounds.Y) * scaleY))

	return PixelRect{
		X:      left,
		Y:      top,
		Width:  maxInt(1, right-left),
		Height: maxInt(1, bottom-top),
	}
}

func logicalRectToGlobalPixelRect(display Display, rect Rect) PixelRect {
	local := logicalRectToDisplayPixelRect(display, rect)
	return PixelRect{
		X:      display.PixelBounds.X + local.X,
		Y:      display.PixelBounds.Y + local.Y,
		Width:  local.Width,
		Height: local.Height,
	}
}

func unionPixelRect(rects []PixelRect) PixelRect {
	if len(rects) == 0 {
		return PixelRect{}
	}

	minX := rects[0].X
	minY := rects[0].Y
	maxRight := rects[0].X + rects[0].Width
	maxBottom := rects[0].Y + rects[0].Height

	for i := 1; i < len(rects); i++ {
		rect := rects[i]
		if rect.X < minX {
			minX = rect.X
		}
		if rect.Y < minY {
			minY = rect.Y
		}
		if rect.X+rect.Width > maxRight {
			maxRight = rect.X + rect.Width
		}
		if rect.Y+rect.Height > maxBottom {
			maxBottom = rect.Y + rect.Height
		}
	}

	return PixelRect{
		X:      minX,
		Y:      minY,
		Width:  maxRight - minX,
		Height: maxBottom - minY,
	}
}

func distance(a Point, b Point) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func clampFloat(value float64, min float64, max float64) float64 {
	if max < min {
		return min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
