// Package filetype reproduces how Adonis's bodyparser names an uploaded
// file's extension: magic-byte detection with the `file-type` package (v20)
// when the client name's extension is one it supports (or there is none),
// otherwise the client name's extension. Detection covers the formats
// people upload here (images, documents, archives, common media);
// TestDetectMatchesFileType compares it with the Node package.
package filetype

import (
	"bytes"
	"encoding/binary"
	"path"
	"regexp"
	"strings"
)

// supported is file-type's supportedExtensions.
var supported = func() map[string]bool {
	m := map[string]bool{}
	for _, e := range strings.Fields(`jpg png apng gif webp flif xcf cr2 cr3 orf arw dng nef rw2 raf tif bmp icns jxr psd indd zip tar rar gz bz2 7z dmg mp4 mid mkv webm mov avi mpg mp2 mp3 m4a oga ogg ogv opus flac wav spx amr pdf epub elf macho exe swf rtf wasm woff woff2 eot ttf otf ttc ico flv ps xz sqlite nes crx xpi cab deb ar rpm Z lz cfb mxf mts blend bpg docx pptx xlsx 3gp 3g2 j2c jp2 jpm jpx mj2 aif qcp odt ods odp xml mobi heic cur ktx ape wv dcm ics glb pcap dsf lnk alias voc ac3 m4v m4p m4b f4v f4p f4b f4a mie asf ogm ogx mpc arrow shp aac mp1 it s3m xm ai skp avif eps lzh pgp asar stl chm 3mf zst jxl vcf jls pst dwg parquet class arj cpio ace avro icc fbx vsdx vtt apk drc lz4 potx xltx dotx xltm ott ots otp odg otg xlsm docm dotm potm pptm jar rm ppsm ppsx`) {
		m[e] = true
	}
	return m
}()

// Type is the bodyparser's view of a file: extension, MIME type and subtype.
type Type struct{ Ext, Type, Subtype string }

// ForUpload is PartHandler#detectFileTypeAndExtension: nil leaves the
// file's extname undefined.
func ForUpload(clientName, contentType string, contents []byte) *Type {
	ext := strings.TrimPrefix(path.Ext(clientName), ".")
	if ext == "" || supported[ext] {
		if t := Detect(contents); t != nil {
			return t
		}
		return nil
	}
	t := &Type{Ext: ext}
	t.Type, t.Subtype = splitMime(contentType)
	return t
}

func splitMime(mime string) (string, string) {
	mime = strings.TrimSpace(strings.SplitN(mime, ";", 2)[0])
	main, sub, ok := strings.Cut(strings.ToLower(mime), "/")
	if !ok || main == "" || sub == "" || strings.ContainsAny(main+sub, " \t") {
		return "", ""
	}
	return main, sub
}

func typed(ext, mime string) *Type {
	t := &Type{Ext: ext}
	t.Type, t.Subtype = splitMime(mime)
	return t
}

func at(b []byte, offset int, sig ...byte) bool {
	return len(b) >= offset+len(sig) && bytes.Equal(b[offset:offset+len(sig)], sig)
}

func str(b []byte, offset int, s string) bool { return at(b, offset, []byte(s)...) }

// Detect is fileTypeFromBuffer for the supported subset.
func Detect(b []byte) *Type {
	switch {
	case len(b) < 2:
		return nil
	case at(b, 0, 0xFF, 0xD8, 0xFF):
		return typed("jpg", "image/jpeg")
	case at(b, 0, 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A):
		if apng(b) {
			return typed("apng", "image/apng")
		}
		return typed("png", "image/png")
	case str(b, 0, "GIF"):
		return typed("gif", "image/gif")
	case str(b, 0, "RIFF") && str(b, 8, "WEBP"):
		return typed("webp", "image/webp")
	case str(b, 0, "RIFF") && str(b, 8, "WAVE"):
		return typed("wav", "audio/wav")
	case str(b, 0, "RIFF") && str(b, 8, "AVI"):
		return typed("avi", "video/vnd.avi")
	case str(b, 0, "BM") && len(b) >= 14:
		return typed("bmp", "image/bmp")
	case str(b, 0, "%PDF"):
		return typed("pdf", "application/pdf")
	case at(b, 0, 0x50, 0x4B, 0x03, 0x04):
		return zipType(b)
	case at(b, 0, 0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1):
		return typed("cfb", "application/x-cfb")
	case str(b, 0, "Rar!\x1a\x07"):
		return typed("rar", "application/x-rar-compressed")
	case at(b, 0, 0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C):
		return typed("7z", "application/x-7z-compressed")
	case at(b, 0, 0x1F, 0x8B, 0x08):
		return typed("gz", "application/gzip")
	case str(b, 0, "II*\x00") || str(b, 0, "MM\x00*"):
		return typed("tif", "image/tiff")
	case str(b, 0, "8BPS"):
		return typed("psd", "image/vnd.adobe.photoshop")
	case str(b, 0, "{\\rtf"):
		return typed("rtf", "application/rtf")
	case str(b, 0, "ID3"):
		return typed("mp3", "audio/mpeg")
	case str(b, 0, "OggS"):
		return typed("ogg", "audio/ogg")
	case str(b, 0, "fLaC"):
		return typed("flac", "audio/flac")
	case at(b, 0, 0x1A, 0x45, 0xDF, 0xA3):
		if bytes.Contains(b[:min(len(b), 4096)], []byte("webm")) {
			return typed("webm", "video/webm")
		}
		return typed("mkv", "video/matroska")
	case at(b, 0, 0x00, 0x00, 0x01, 0x00) && len(b) >= 6:
		return typed("ico", "image/x-icon")
	case str(b, 0, "SQLite format 3\x00"):
		return typed("sqlite", "application/x-sqlite3")
	case str(b, 0, "<?xml "):
		return typed("xml", "application/xml")
	case len(b) >= 12 && str(b, 4, "ftyp"):
		return ftyp(b)
	}
	return nil
}

func apng(b []byte) bool {
	for i := 8; i+8 <= len(b); {
		length := int(binary.BigEndian.Uint32(b[i:]))
		kind := string(b[i+4 : i+8])
		if kind == "IDAT" {
			return false
		}
		if kind == "acTL" {
			return true
		}
		i += 12 + length
	}
	return false
}

func ftyp(b []byte) *Type {
	brand := strings.TrimRight(strings.ReplaceAll(string(b[8:12]), "\x00", " "), " ")
	switch {
	case brand == "avif" || brand == "avis":
		return typed("avif", "image/avif")
	case brand == "mif1" || brand == "msf1" || brand == "heic" || brand == "heix" || brand == "hevc" || brand == "hevx":
		return typed("heic", "image/heic")
	case brand == "qt":
		return typed("mov", "video/quicktime")
	case brand == "M4A":
		return typed("m4a", "audio/x-m4a")
	case brand == "M4V" || brand == "M4VH" || brand == "M4VP":
		return typed("m4v", "video/x-m4v")
	case strings.HasPrefix(brand, "3g2"):
		return typed("3g2", "video/3gpp2")
	case strings.HasPrefix(brand, "3g"):
		return typed("3gp", "video/3gpp")
	}
	return typed("mp4", "video/mp4")
}

var zipMimeTypes = map[string][2]string{
	"application/epub+zip":                                                      {"epub", "application/epub+zip"},
	"application/vnd.oasis.opendocument.text":                                   {"odt", "application/vnd.oasis.opendocument.text"},
	"application/vnd.oasis.opendocument.text-template":                          {"ott", "application/vnd.oasis.opendocument.text-template"},
	"application/vnd.oasis.opendocument.spreadsheet":                            {"ods", "application/vnd.oasis.opendocument.spreadsheet"},
	"application/vnd.oasis.opendocument.spreadsheet-template":                   {"ots", "application/vnd.oasis.opendocument.spreadsheet-template"},
	"application/vnd.oasis.opendocument.presentation":                           {"odp", "application/vnd.oasis.opendocument.presentation"},
	"application/vnd.oasis.opendocument.presentation-template":                  {"otp", "application/vnd.oasis.opendocument.presentation-template"},
	"application/vnd.oasis.opendocument.graphics":                               {"odg", "application/vnd.oasis.opendocument.graphics"},
	"application/vnd.oasis.opendocument.graphics-template":                      {"otg", "application/vnd.oasis.opendocument.graphics-template"},
	"application/vnd.openxmlformats-officedocument.presentationml.slideshow":    {"ppsx", "application/vnd.openxmlformats-officedocument.presentationml.slideshow"},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {"xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	"application/vnd.ms-excel.sheet.macroenabled":                               {"xlsm", "application/vnd.ms-excel.sheet.macroenabled.12"},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.template":      {"xltx", "application/vnd.openxmlformats-officedocument.spreadsheetml.template"},
	"application/vnd.ms-excel.template.macroenabled":                            {"xltm", "application/vnd.ms-excel.template.macroenabled.12"},
	"application/vnd.ms-powerpoint.slideshow.macroenabled":                      {"ppsm", "application/vnd.ms-powerpoint.slideshow.macroenabled.12"},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   {"docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"application/vnd.ms-word.document.macroenabled":                             {"docm", "application/vnd.ms-word.document.macroenabled.12"},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.template":   {"dotx", "application/vnd.openxmlformats-officedocument.wordprocessingml.template"},
	"application/vnd.ms-word.template.macroenabledtemplate":                     {"dotm", "application/vnd.ms-word.template.macroenabled.12"},
	"application/vnd.openxmlformats-officedocument.presentationml.template":     {"potx", "application/vnd.openxmlformats-officedocument.presentationml.template"},
	"application/vnd.ms-powerpoint.template.macroenabled":                       {"potm", "application/vnd.ms-powerpoint.template.macroenabled.12"},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {"pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	"application/vnd.ms-powerpoint.presentation.macroenabled":                   {"pptm", "application/vnd.ms-powerpoint.presentation.macroenabled.12"},
	"application/vnd.ms-visio.drawing":                                          {"vsdx", "application/vnd.visio"},
	"application/vnd.ms-package.3dmanufacturing-3dmodel+xml":                    {"3mf", "model/3mf"},
}

func fromMime(mime string) *Type {
	if t, ok := zipMimeTypes[strings.ToLower(mime)]; ok {
		return typed(t[0], t[1])
	}
	return nil
}

var dexEntry = regexp.MustCompile(`classes\d*\.dex`)

// zipType is file-type's ZipHandler walk: specific entries identify the
// format, `mimetype` and `[Content_Types].xml` are read and end the walk.
func zipType(b []byte) *Type {
	zipped := typed("zip", "application/zip")
	for i := 0; i+30 <= len(b) && at(b, i, 0x50, 0x4B, 0x03, 0x04); {
		nameLength := int(binary.LittleEndian.Uint16(b[i+26:]))
		extraLength := int(binary.LittleEndian.Uint16(b[i+28:]))
		compressed := int(binary.LittleEndian.Uint32(b[i+18:]))
		if i+30+nameLength > len(b) {
			break
		}
		name := string(b[i+30 : i+30+nameLength])
		dataStart := i + 30 + nameLength + extraLength
		flags := binary.LittleEndian.Uint16(b[i+6:])
		if flags&0x08 != 0 && compressed == 0 && dataStart <= len(b) {
			// Sizes follow in a data descriptor: the entry runs to the next signature.
			next := -1
			for j := dataStart; j+4 <= len(b); j++ {
				if at(b, j, 0x50, 0x4B, 0x07, 0x08) || at(b, j, 0x50, 0x4B, 0x03, 0x04) || at(b, j, 0x50, 0x4B, 0x01, 0x02) {
					next = j - dataStart
					break
				}
			}
			if next < 0 {
				break
			}
			compressed = next
		}
		data := func() []byte {
			if dataStart+compressed > len(b) {
				return b[min(dataStart, len(b)):]
			}
			return b[dataStart : dataStart+compressed]
		}
		switch {
		case name == "META-INF/mozilla.rsa":
			return typed("xpi", "application/x-xpinstall")
		case name == "META-INF/MANIFEST.MF":
			return typed("jar", "application/java-archive")
		case name == "mimetype":
			if t := fromMime(strings.TrimSpace(string(data()))); t != nil {
				return t
			}
			return zipped
		case name == "[Content_Types].xml":
			xml := string(data())
			end := strings.Index(xml, `.main+xml"`)
			if end == -1 {
				if strings.Contains(xml, `ContentType="application/vnd.ms-package.3dmanufacturing-3dmodel+xml"`) {
					return fromMime("application/vnd.ms-package.3dmanufacturing-3dmodel+xml")
				}
				return zipped
			}
			head := xml[:end]
			if t := fromMime(head[strings.LastIndex(head, `"`)+1:]); t != nil {
				return t
			}
			return zipped
		case dexEntry.MatchString(name):
			return typed("apk", "application/vnd.android.package-archive")
		}
		i = dataStart + compressed
		if flags&0x08 != 0 && at(b, i, 0x50, 0x4B, 0x07, 0x08) {
			i += 16
		}
	}
	return zipped
}
