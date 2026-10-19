// Package pdfcheck answers the question web-be's form_file_service.ts asks
// pdf-lib: does `PDFDocument.load(bytes)` succeed and report at least one
// page? It reads indirect objects leniently (as pdf-lib does, without
// trusting the cross-reference table), unpacks object streams, refuses
// encrypted documents, and counts the leaves of the catalog's page tree.
package pdfcheck

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strconv"
)

var (
	objectHeader = regexp.MustCompile(`(\d+)\s+(\d+)\s+obj\b`)
	refPattern   = regexp.MustCompile(`(\d+)\s+(\d+)\s+R\b`)
	rootPattern  = regexp.MustCompile(`/Root\s+(\d+)\s+(\d+)\s+R\b`)
	typePattern  = regexp.MustCompile(`/Type\s*/(\w+)`)
	kidsPattern  = regexp.MustCompile(`/Kids\s*\[([^\]]*)\]`)
	pagesPattern = regexp.MustCompile(`/Pages\s+(\d+)\s+(\d+)\s+R\b`)
	firstPattern = regexp.MustCompile(`/First\s+(\d+)`)
	countPattern = regexp.MustCompile(`/N\s+(\d+)`)
)

type document struct {
	objects map[int][]byte // object number -> body (last definition wins)
	roots   [][]byte
	broken  bool // an object runs into the end of the file without endobj
}

// HasPages reports whether pdf-lib would load the document and find at
// least one page.
func HasPages(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return false
	}
	doc := parse(data)
	if doc.broken || doc.encrypted(data) {
		return false
	}
	root := doc.catalog()
	if root == nil {
		return false
	}
	m := pagesPattern.FindSubmatch(root)
	if m == nil {
		return false
	}
	n, _ := strconv.Atoi(string(m[1]))
	count, ok := doc.countPages(n, map[int]bool{})
	return ok && count > 0
}

func (d *document) encrypted(data []byte) bool {
	for _, trailer := range d.roots {
		if bytes.Contains(trailer, []byte("/Encrypt")) {
			return true
		}
	}
	return false
}

func parse(data []byte) *document {
	d := &document{objects: map[int][]byte{}}
	for _, loc := range objectHeader.FindAllSubmatchIndex(data, -1) {
		n, _ := strconv.Atoi(string(data[loc[2]:loc[3]]))
		start := loc[1]
		end := bytes.Index(data[start:], []byte("endobj"))
		if end < 0 {
			// pdf-lib fails to parse an unterminated object.
			d.broken = true
			break
		}
		body := data[start : start+end]
		d.objects[n] = body
		if typ := typePattern.FindSubmatch(dictionary(body)); typ != nil {
			switch string(typ[1]) {
			case "ObjStm":
				d.unpack(body)
			case "XRef":
				d.roots = append(d.roots, dictionary(body))
			}
		}
	}
	for _, i := range allIndexes(data, []byte("trailer")) {
		d.roots = append(d.roots, data[i:min(len(data), i+2048)])
	}
	return d
}

func allIndexes(data, needle []byte) []int {
	var out []int
	for i := 0; ; {
		j := bytes.Index(data[i:], needle)
		if j < 0 {
			return out
		}
		out = append(out, i+j)
		i += j + len(needle)
	}
}

// dictionary is the object's dictionary text (before any stream data).
func dictionary(body []byte) []byte {
	if i := bytes.Index(body, []byte("stream")); i >= 0 {
		return body[:i]
	}
	return body
}

func streamData(body []byte) []byte {
	i := bytes.Index(body, []byte("stream"))
	if i < 0 {
		return nil
	}
	data := body[i+len("stream"):]
	data = bytes.TrimLeft(data, "\r\n")
	if j := bytes.LastIndex(data, []byte("endstream")); j >= 0 {
		data = data[:j]
	}
	return data
}

// unpack adds the objects of an object stream (FlateDecode).
func (d *document) unpack(body []byte) {
	dict := dictionary(body)
	if !bytes.Contains(dict, []byte("FlateDecode")) {
		return
	}
	first := firstPattern.FindSubmatch(dict)
	count := countPattern.FindSubmatch(dict)
	if first == nil || count == nil {
		return
	}
	r, err := zlib.NewReader(bytes.NewReader(streamData(body)))
	if err != nil {
		return
	}
	plain, _ := io.ReadAll(r)
	firstOffset, _ := strconv.Atoi(string(first[1]))
	n, _ := strconv.Atoi(string(count[1]))
	if firstOffset > len(plain) {
		return
	}
	header := bytes.Fields(plain[:firstOffset])
	type entry struct{ number, offset int }
	var entries []entry
	for i := 0; i+1 < len(header) && len(entries) < n; i += 2 {
		number, err1 := strconv.Atoi(string(header[i]))
		offset, err2 := strconv.Atoi(string(header[i+1]))
		if err1 != nil || err2 != nil {
			return
		}
		entries = append(entries, entry{number, offset})
	}
	for i, e := range entries {
		start := firstOffset + e.offset
		end := len(plain)
		if i+1 < len(entries) {
			end = firstOffset + entries[i+1].offset
		}
		if start <= end && end <= len(plain) {
			if _, exists := d.objects[e.number]; !exists {
				d.objects[e.number] = plain[start:end]
			}
		}
	}
}

func (d *document) catalog() []byte {
	for i := len(d.roots) - 1; i >= 0; i-- {
		if m := rootPattern.FindSubmatch(d.roots[i]); m != nil {
			n, _ := strconv.Atoi(string(m[1]))
			if body, ok := d.objects[n]; ok {
				return dictionary(body)
			}
		}
	}
	// pdf-lib recovers a missing /Root by looking for a catalog object.
	for _, body := range d.objects {
		if typ := typePattern.FindSubmatch(dictionary(body)); typ != nil && string(typ[1]) == "Catalog" {
			return dictionary(body)
		}
	}
	return nil
}

func (d *document) countPages(n int, seen map[int]bool) (int, bool) {
	if seen[n] {
		return 0, false
	}
	seen[n] = true
	body, ok := d.objects[n]
	if !ok {
		return 0, false
	}
	dict := dictionary(body)
	typ := typePattern.FindSubmatch(dict)
	if typ != nil && string(typ[1]) == "Page" {
		return 1, true
	}
	kids := kidsPattern.FindSubmatch(dict)
	if kids == nil {
		return 0, typ != nil && string(typ[1]) == "Pages"
	}
	total := 0
	for _, ref := range refPattern.FindAllSubmatch(kids[1], -1) {
		child, _ := strconv.Atoi(string(ref[1]))
		c, ok := d.countPages(child, seen)
		if !ok {
			return 0, false
		}
		total += c
	}
	return total, true
}
