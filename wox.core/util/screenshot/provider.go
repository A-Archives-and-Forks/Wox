package screenshot

import (
	"fmt"
	"wox/util/screen"
)

func listDisplaysFromSystem() ([]Display, error) {
	displays, err := screen.ListDisplays()
	if err != nil {
		return nil, err
	}

	result := make([]Display, 0, len(displays))
	for _, display := range displays {
		result = append(result, Display{
			ID:   display.ID,
			Name: display.Name,
			Bounds: Rect{
				X:      float64(display.Bounds.X),
				Y:      float64(display.Bounds.Y),
				Width:  float64(display.Bounds.Width),
				Height: float64(display.Bounds.Height),
			},
			PixelBounds: PixelRect{
				X:      display.PixelBounds.X,
				Y:      display.PixelBounds.Y,
				Width:  display.PixelBounds.Width,
				Height: display.PixelBounds.Height,
			},
			Scale:   display.Scale,
			Primary: display.Primary,
		})
	}

	return result, nil
}

func getPixelBoundsForDisplays(displays []Display, ids []string) (PixelRect, error) {
	if len(displays) == 0 {
		return PixelRect{}, ErrNoDisplays
	}

	if len(ids) == 0 {
		ids = make([]string, 0, len(displays))
		for _, display := range displays {
			ids = append(ids, display.ID)
		}
	}

	selected := filterDisplays(displays, ids)
	if len(selected) == 0 {
		return PixelRect{}, fmt.Errorf("no displays matched requested ids")
	}

	minX := selected[0].PixelBounds.X
	minY := selected[0].PixelBounds.Y
	maxRight := selected[0].PixelBounds.X + selected[0].PixelBounds.Width
	maxBottom := selected[0].PixelBounds.Y + selected[0].PixelBounds.Height

	for i := 1; i < len(selected); i++ {
		bounds := selected[i].PixelBounds
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

	return PixelRect{
		X:      minX,
		Y:      minY,
		Width:  maxRight - minX,
		Height: maxBottom - minY,
	}, nil
}
