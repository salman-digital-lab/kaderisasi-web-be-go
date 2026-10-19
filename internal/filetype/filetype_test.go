package filetype

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os/exec"
	"testing"
)

func zipOf(t *testing.T, entries ...[2]string) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e[0], Method: zip.Store}
		f, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(e[1]))
	}
	_ = w.Close()
	return b.Bytes()
}

func samples(t *testing.T) map[string][]byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var p, j, g bytes.Buffer
	_ = png.Encode(&p, img)
	_ = jpeg.Encode(&j, img, nil)
	_ = gif.Encode(&g, img, nil)
	return map[string][]byte{
		"png": p.Bytes(), "jpg": j.Bytes(), "gif": g.Bytes(),
		"webp":       append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 \x18\x00\x00\x00"), make([]byte, 24)...),
		"pdf":        []byte("%PDF-1.7\n1 0 obj << >> endobj\ntrailer << >>\n%%EOF\n"),
		"docx":       zipOf(t, [2]string{"[Content_Types].xml", `<Types><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`}, [2]string{"word/document.xml", "<w/>"}),
		"xlsx":       zipOf(t, [2]string{"[Content_Types].xml", `<Types><Override ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`}),
		"pptx":       zipOf(t, [2]string{"[Content_Types].xml", `<Types><Override ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/></Types>`}),
		"ooxml-bare": zipOf(t, [2]string{"[Content_Types].xml", "<Types/>"}, [2]string{"word/document.xml", "<w/>"}),
		"jar":        zipOf(t, [2]string{"META-INF/MANIFEST.MF", "Manifest-Version: 1.0"}),
		"odt":        zipOf(t, [2]string{"mimetype", "application/vnd.oasis.opendocument.text"}, [2]string{"content.xml", "<x/>"}),
		"epub":       zipOf(t, [2]string{"mimetype", "application/epub+zip"}, [2]string{"META-INF/container.xml", "<x/>"}),
		"zip":        zipOf(t, [2]string{"a.txt", "hello"}, [2]string{"b/c.txt", "world"}),
		"cfb":        append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 512)...),
		"gz":         {0x1F, 0x8B, 0x08, 0, 0, 0, 0, 0, 0, 3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		"7z":         append([]byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C, 0, 4}, make([]byte, 24)...),
		"rtf":        []byte(`{\rtf1\ansi Hello}`),
		"text":       []byte("just some text\nline 2"),
		"html":       []byte("<!doctype html><html><body>x</body></html>"),
		"svg":        []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"empty":      {},
		"random":     {0x13, 0x37, 0x42, 0x99, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
		"mp4":        append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), make([]byte, 16)...),
		"heic":       append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), make([]byte, 16)...),
		"mov":        append([]byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00qt  "), make([]byte, 16)...),
		"psd":        append([]byte("8BPS\x00\x01"), make([]byte, 30)...),
	}
}

func TestDetectMatchesFileType(t *testing.T) {
	cases := samples(t)
	names := []string{}
	inputs := []string{}
	for name, b := range cases {
		names = append(names, name)
		inputs = append(inputs, base64.StdEncoding.EncodeToString(b))
	}
	raw, _ := json.Marshal(inputs)
	command := exec.Command("node", "../../scripts/filetype-fixture.mjs")
	command.Stdin = bytes.NewReader(raw)
	out, err := command.Output()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var want []*struct{ Ext, Mime string }
	if err := json.Unmarshal(out, &want); err != nil {
		t.Fatal(err)
	}
	for i, name := range names {
		got := Detect(cases[name])
		switch {
		case want[i] == nil && got != nil:
			t.Errorf("%s: got %s, file-type finds nothing", name, got.Ext)
		case want[i] != nil && (got == nil || got.Ext != want[i].Ext || got.Type+"/"+got.Subtype != want[i].Mime):
			t.Errorf("%s: got %+v, file-type says %+v", name, got, *want[i])
		}
	}
}

func TestForUpload(t *testing.T) {
	jpg := samples(t)["jpg"]
	if got := ForUpload("photo.png", "image/png", jpg); got == nil || got.Ext != "jpg" {
		t.Fatalf("detected type wins for a supported extension: %+v", got)
	}
	if got := ForUpload("photo.JPG", "image/jpeg", []byte("not an image")); got == nil || got.Ext != "JPG" || got.Type != "image" {
		t.Fatalf("unsupported extension falls back to the name: %+v", got)
	}
	if got := ForUpload("notes.png", "image/png", []byte("text")); got != nil {
		t.Fatalf("undetectable content leaves extname undefined: %+v", got)
	}
}
