package screenshot

import (
	"image"
	"image/draw"
)

type RenderInput struct {
	BaseImage image.Image
	Document  Document
	Selection Rect
}

type Renderer interface {
	Render(input RenderInput) (*image.RGBA, error)
}

type defaultRenderer struct{}

func newRenderer() Renderer {
	return &defaultRenderer{}
}

func (r *defaultRenderer) Render(input RenderInput) (*image.RGBA, error) {
	if input.BaseImage == nil {
		return nil, ErrNoImage
	}

	return toRGBA(input.BaseImage), nil
}

func toRGBA(src image.Image) *image.RGBA {
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}

	bounds := src.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, bounds.Min, draw.Src)
	return rgba
}
