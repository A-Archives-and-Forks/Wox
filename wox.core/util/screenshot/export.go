package screenshot

import (
	"image"
	"image/png"
	"os"

	"wox/util/clipboard"
)

type ExportRequest struct {
	Targets   []ExportTarget
	Image     image.Image
	Selection Rect
	Document  Document
	TempDir   string
}

type ExportResult struct {
	FilePath string
}

type Exporter interface {
	Export(req ExportRequest) (*ExportResult, error)
}

type defaultExporter struct{}

func newExporter() Exporter {
	return &defaultExporter{}
}

func (e *defaultExporter) Export(req ExportRequest) (*ExportResult, error) {
	if req.Image == nil {
		return nil, ErrNoImage
	}

	if hasExportTarget(req.Targets, ExportTargetClipboard) {
		if err := clipboard.Write(&clipboard.ImageData{Image: req.Image}); err != nil {
			return nil, err
		}
	}

	result := &ExportResult{}
	if hasExportTarget(req.Targets, ExportTargetTempFile) {
		file, err := os.CreateTemp(req.TempDir, "wox-screenshot-*.png")
		if err != nil {
			return nil, err
		}
		defer file.Close()

		if err := png.Encode(file, req.Image); err != nil {
			return nil, err
		}
		result.FilePath = file.Name()
	}

	return result, nil
}

func hasExportTarget(targets []ExportTarget, target ExportTarget) bool {
	for _, current := range targets {
		if current == target {
			return true
		}
	}

	return false
}
