package forms

import (
	"errors"
	"strings"
	"unicode/utf16"

	"kaderisasi/web/internal/media"
	"kaderisasi/web/internal/pdfcheck"
)

// Processed is a ProcessedFormFile.
type Processed struct {
	Contents      []byte
	MimeType, Ext string
	Width, Height *int32
}

// ProcessFile is processFormFile; errors carry the message web-be reports.
func ProcessFile(input []byte, s FileSettings) (Processed, error) {
	if len(input) == 0 || len(input) > s.MaxSizeMB*1024*1024 {
		return Processed{}, errors.New("INVALID_FILE_SIZE")
	}
	if len(input) >= 5 && string(input[:5]) == "%PDF-" {
		if s.Accept == "image" {
			return Processed{}, errors.New("FILE_TYPE_NOT_ALLOWED")
		}
		if !pdfcheck.HasPages(input) {
			return Processed{}, errors.New("INVALID_PDF")
		}
		return Processed{Contents: input, MimeType: "application/pdf", Ext: "pdf"}, nil
	}
	if s.Accept == "pdf" {
		return Processed{}, errors.New("FILE_TYPE_NOT_ALLOWED")
	}
	if media.HasAnimation(input) {
		return Processed{}, errors.New("INVALID_IMAGE")
	}
	out, w, h, err := media.OptimizeWithSize(input, media.FormImage)
	if err != nil {
		return Processed{}, errors.New("INVALID_IMAGE")
	}
	width, height := int32(w), int32(h)
	return Processed{Contents: out, MimeType: "image/webp", Ext: "webp", Width: &width, Height: &height}, nil
}

// OriginalName is the upload's display name: the basename without control
// characters, at most 200 UTF-16 units, or "berkas".
func OriginalName(clientName string) string {
	base := strings.TrimRight(clientName, "/") // path.basename ignores trailing separators
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	var kept []rune
	for _, r := range base {
		unit := r
		if r >= 0x10000 {
			unit = 0xD800 // a surrogate's first unit is never a control character
		}
		if unit >= 32 && unit != 127 {
			kept = append(kept, r)
		}
	}
	units := utf16.Encode(kept)
	if len(units) > 200 {
		units = units[:200]
	}
	name := string(utf16.Decode(units))
	if name == "" {
		return "berkas"
	}
	return name
}

// DownloadName is `originalName.replace(/\.[^.]+$/, ”) + '.' + extension`.
func DownloadName(original, ext string) string {
	if i := strings.LastIndex(original, "."); i >= 0 && i < len(original)-1 {
		original = original[:i]
	}
	return original + "." + ext
}
