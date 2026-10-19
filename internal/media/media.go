// Package media reproduces web-be's sharp pipelines with libvips (govips):
// the profile picture optimiser (profile_image_service.ts). libvips builds
// differ, so output bytes are equivalent images, not identical files.
package media

import (
	"errors"
	"math"
	"sync"

	"github.com/davidbyttow/govips/v2/vips"
)

// ErrInvalidImage is InvalidProfileImageError.
var ErrInvalidImage = errors.New("INVALID_PICTURE")

var startup sync.Once

func start() {
	startup.Do(func() {
		vips.LoggingSettings(nil, vips.LogLevelError)
		vips.Startup(&vips.Config{ConcurrencyLevel: 2})
	})
}

// Format is a sharp output format.
type Format string

const (
	WebP Format = "webp"
	JPEG Format = "jpeg"
	PNG  Format = "png"
)

// Options mirror the sharp calls web-be makes.
type Options struct {
	MaxInputPixels int64
	Width, Height  int
	Format         Format
	Quality        int
}

// ProfilePicture is optimizeProfileImage: JPEG/PNG/WebP single-page input
// up to 40 MP, auto-oriented, fitted inside 512x512 without enlargement,
// re-encoded as WebP quality 82 / effort 4 without metadata.
var ProfilePicture = Options{MaxInputPixels: 40_000_000, Width: 512, Height: 512, Format: WebP, Quality: 82}

// FormImage is form_file_service.ts's image branch: up to 2400x2400, WebP
// quality 85 / effort 4.
var FormImage = Options{MaxInputPixels: 40_000_000, Width: 2400, Height: 2400, Format: WebP, Quality: 85}

// Optimize runs the pipeline; any failure is ErrInvalidImage.
func Optimize(data []byte, o Options) ([]byte, error) {
	out, _, _, err := OptimizeWithSize(data, o)
	return out, err
}

// OptimizeWithSize also reports the output dimensions.
func OptimizeWithSize(data []byte, o Options) ([]byte, int, int, error) {
	out, w, h, err := optimize(data, o)
	if err != nil {
		return nil, 0, 0, ErrInvalidImage
	}
	return out, w, h, nil
}

func optimize(data []byte, o Options) ([]byte, int, int, error) {
	start()
	img, err := vips.NewImageFromBuffer(data)
	if err != nil {
		return nil, 0, 0, ErrInvalidImage
	}
	defer img.Close()
	format := img.OriginalFormat()
	if (format != vips.ImageTypeJPEG && format != vips.ImageTypePNG && format != vips.ImageTypeWEBP) || img.Pages() > 1 {
		return nil, 0, 0, ErrInvalidImage
	}
	if img.Width() <= 0 || img.Height() <= 0 || int64(img.Width())*int64(img.Height()) > o.MaxInputPixels {
		return nil, 0, 0, ErrInvalidImage
	}
	if err := img.AutoRotate(); err != nil {
		return nil, 0, 0, ErrInvalidImage
	}
	// fit: 'inside' with withoutEnlargement: never upscale.
	scale := math.Min(1, math.Min(float64(o.Width)/float64(img.Width()), float64(o.Height)/float64(img.Height())))
	if scale < 1 {
		if err := img.Resize(scale, vips.KernelLanczos3); err != nil {
			return nil, 0, 0, ErrInvalidImage
		}
	}
	var out []byte
	switch o.Format {
	case JPEG:
		out, _, err = img.ExportJpeg(&vips.JpegExportParams{Quality: o.Quality, StripMetadata: true})
	case PNG:
		out, _, err = img.ExportPng(&vips.PngExportParams{Compression: 9, StripMetadata: true})
	default:
		out, _, err = img.ExportWebp(&vips.WebpExportParams{Quality: o.Quality, ReductionEffort: 4, StripMetadata: true})
	}
	if err != nil {
		return nil, 0, 0, ErrInvalidImage
	}
	return out, img.Width(), img.Height(), nil
}

// HasAnimation is form_file_service.ts hasAnimation: an animated PNG or WebP.
func HasAnimation(input []byte) bool {
	png := len(input) >= 8 && string(input[:8]) == "\x89PNG\r\n\x1a\n"
	webp := len(input) >= 12 && string(input[:4]) == "RIFF" && string(input[8:12]) == "WEBP"
	if !png && !webp {
		return false
	}
	offset := 12
	if png {
		offset = 8
	}
	for offset+8 <= len(input) {
		var length int
		var kind string
		if png {
			length = int(uint32(input[offset])<<24 | uint32(input[offset+1])<<16 | uint32(input[offset+2])<<8 | uint32(input[offset+3]))
			kind = string(input[offset+4 : offset+8])
		} else {
			length = int(uint32(input[offset+4]) | uint32(input[offset+5])<<8 | uint32(input[offset+6])<<16 | uint32(input[offset+7])<<24)
			kind = string(input[offset : offset+4])
		}
		if kind == "acTL" || kind == "ANIM" || kind == "ANMF" {
			return true
		}
		if png {
			offset += length + 12
		} else {
			offset += length + 8 + length%2
		}
	}
	return false
}
