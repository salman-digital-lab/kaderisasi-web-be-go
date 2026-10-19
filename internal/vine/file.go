package vine

import (
	"math"
	"strconv"
	"strings"

	"kaderisasi/web/internal/js"
)

// Upload is a multipart file as Adonis's bodyparser describes it.
type Upload struct {
	FieldName, ClientName, ContentType string
	Data                               []byte
	// Extname, Type and Subtype come from content detection (filetype);
	// HasExtname is false when it is undefined.
	Extname, Type, Subtype string
	HasExtname             bool
	Errors                 []FileError
}

// FileError is one entry of MultipartFile#errors.
type FileError struct {
	FieldName  string `json:"fieldName"`
	ClientName string `json:"clientName"`
	Message    string `json:"message"`
	Type       string `json:"type"`
}

// File is `vine.file({ size, extnames })` from @adonisjs/core.
type File struct {
	base
	Size     string // e.g. "2mb"
	Extnames []string
}

func (s File) setOptional() File { s.Optional = true; return s }

// OptionalFile returns s marked optional.
func OptionalFile(s File) File { return s.setOptional() }

// ParseBytes is the `bytes` package's parse for "<n><unit>" strings.
func ParseBytes(v string) float64 {
	units := map[string]float64{"b": 1, "kb": 1 << 10, "mb": 1 << 20, "gb": 1 << 30, "tb": 1 << 40, "pb": 1 << 50}
	lower := strings.ToLower(strings.TrimSpace(v))
	for _, u := range []string{"kb", "mb", "gb", "tb", "pb", "b"} {
		if strings.HasSuffix(lower, u) {
			n, err := strconv.ParseFloat(strings.TrimSpace(lower[:len(lower)-len(u)]), 64)
			if err == nil {
				return math.Floor(units[u] * n)
			}
		}
	}
	return js.ParseIntString(v)
}

// FormatBytes is the `bytes` package's format with its defaults: two
// decimals, trailing zeros removed.
func FormatBytes(n float64) string {
	units := []struct {
		name string
		size float64
	}{{"PB", 1 << 50}, {"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}}
	for _, u := range units {
		if math.Abs(n) >= u.size || u.name == "B" {
			s := strconv.FormatFloat(n/u.size, 'f', 2, 64)
			if strings.Contains(s, ".") {
				s = strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
			}
			return s + u.name
		}
	}
	return ""
}

// Validate runs MultipartFile#validate with the rule's options: the
// extension check, then the size check.
func (s File) check(u *Upload) {
	if len(s.Extnames) > 0 {
		ext := ""
		if u.HasExtname {
			ext = u.Extname
		}
		ok := false
		for _, allowed := range s.Extnames {
			ok = ok || allowed == ext
		}
		if !ok {
			shown := "undefined"
			if u.HasExtname {
				shown = u.Extname
			}
			verb := "are"
			if len(s.Extnames) == 1 {
				verb = "is"
			}
			u.Errors = append(u.Errors, FileError{u.FieldName, u.ClientName, "Invalid file extension " + shown + ". Only " + strings.Join(s.Extnames, ", ") + " " + verb + " allowed", "extname"})
		}
	}
	if s.Size != "" {
		limit := ParseBytes(s.Size)
		if float64(len(u.Data)) > limit {
			u.Errors = append(u.Errors, FileError{u.FieldName, u.ClientName, "File size should be less than " + FormatBytes(limit), "size"})
		}
	}
}

func (s File) validate(f *field) (js.Value, bool) {
	u, ok := f.value.File.(*Upload)
	if f.value.Kind != 'F' || !ok {
		f.reporter.report(f, "The {{ field }} must be a file", "file", nil)
		return js.Value{}, false
	}
	s.check(u)
	if len(u.Errors) == 0 {
		return f.value, true
	}
	// meta is the rule's options object, in its declared order.
	meta := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	if s.Size != "" {
		meta.Keys = append(meta.Keys, "size")
		meta.Fields["size"] = js.Value{Kind: 's', String: s.Size}
	}
	if s.Extnames != nil {
		list := js.Value{Kind: 'a', Items: []js.Value{}}
		for _, e := range s.Extnames {
			list.Items = append(list.Items, js.Value{Kind: 's', String: e})
		}
		meta.Keys = append(meta.Keys, "extnames")
		meta.Fields["extnames"] = list
	}
	for _, e := range u.Errors {
		f.reporter.reportMeta(f, e.Message, "file."+e.Type, meta)
	}
	return js.Value{}, false
}

// Valid is MultipartFile#isValid after validating with the rule's options;
// u itself is not modified.
func (s File) Valid(u *Upload) bool {
	c := *u
	c.Errors = append([]FileError(nil), u.Errors...)
	s.check(&c)
	return len(c.Errors) == 0
}
