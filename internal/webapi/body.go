package webapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"kaderisasi/web/internal/filetype"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/vine"
)

// bodyError is a request body the Adonis bodyparser rejects before the
// controller runs.
type bodyError struct {
	status  int
	name    string
	message string
}

func (e *bodyError) Error() string { return e.message }

var strictJSON = regexp.MustCompile(`^[\x20\x09\x0a\x0d]*(\[|\{)`)

// Limits from kaderisasi-web-be/config/bodyparser.ts defaults.
const jsonLimit = 1 << 20
const formLimit = 56 << 10
const multipartLimit = 20 << 20

// all is Adonis's request.all(): `{...request.qs(), ...request.body()}` as an
// ordered JavaScript object. The bodyparser turns empty body strings into
// null (convertEmptyStringsToNull); query string values are kept as they are.
func all(r *http.Request) (js.Value, error) {
	out := orderedQuery(r.URL.RawQuery, false)
	body, err := parseBody(r)
	if err != nil {
		return js.Value{}, err
	}
	if body.Kind == 'o' {
		for _, k := range body.Keys {
			put(&out, k, body.Fields[k])
		}
	} else if body.Kind == 'a' {
		for i, item := range body.Items {
			put(&out, strconv.Itoa(i), item)
		}
	}
	out.Keys = js.ObjectKeyOrder(out.Keys)
	return out, nil
}

func put(o *js.Value, k string, v js.Value) {
	if _, seen := o.Fields[k]; !seen {
		o.Keys = append(o.Keys, k)
	}
	o.Fields[k] = v
}

// orderedQuery parses a query string like `qs` for flat keys: a repeated
// key becomes an array.
func orderedQuery(raw string, emptyToNull bool) js.Value {
	out := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		key, err := url.QueryUnescape(k)
		if err != nil {
			key = k
		}
		value, err := url.QueryUnescape(v)
		if err != nil {
			value = v
		}
		item := js.Value{Kind: 's', String: value}
		if emptyToNull && value == "" {
			item = js.Value{Kind: 'n'}
		}
		if existing, ok := out.Fields[key]; ok {
			if existing.Kind == 'a' {
				existing.Items = append(existing.Items, item)
				out.Fields[key] = existing
			} else {
				out.Fields[key] = js.Value{Kind: 'a', Items: []js.Value{existing, item}}
			}
			continue
		}
		put(&out, key, item)
	}
	out.Keys = js.ObjectKeyOrder(out.Keys)
	return out
}

func parseBody(r *http.Request) (js.Value, error) {
	none := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return none, nil
	}
	if r.Body == nil || (r.ContentLength == 0 && len(r.TransferEncoding) == 0) {
		return none, nil
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch mediaType {
	case "application/json", "application/json-patch+json", "application/vnd.api+json", "application/csp-report":
		raw, err := readLimited(r.Body, jsonLimit)
		if err != nil {
			return none, err
		}
		if len(raw) == 0 {
			return none, nil
		}
		if !strictJSON.Match(raw) {
			return none, &bodyError{http.StatusUnprocessableEntity, "Exception", "Invalid JSON, only supports object and array"}
		}
		parsed, err := js.Parse(raw)
		if err != nil {
			// JSON.parse's SyntaxError; V8's wording is only reproduced for a
			// truncated document.
			text := "Invalid JSON body"
			var decoder = json.NewDecoder(bytes.NewReader(raw))
			var probe any
			if derr := decoder.Decode(&probe); errors.Is(derr, io.ErrUnexpectedEOF) {
				text = "Unexpected end of JSON input"
			}
			return none, &bodyError{http.StatusBadRequest, "SyntaxError", text}
		}
		return emptyToNull(parsed), nil
	case "multipart/form-data":
		return parseMultipart(r)
	case "application/x-www-form-urlencoded":
		raw, err := readLimited(r.Body, formLimit)
		if err != nil {
			return none, err
		}
		return orderedQuery(string(raw), true), nil
	}
	return none, nil
}

func readLimited(body io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, &bodyError{http.StatusBadRequest, "BadRequestError", "request aborted"}
	}
	if int64(len(raw)) > limit {
		return nil, &bodyError{http.StatusRequestEntityTooLarge, "PayloadTooLargeError", "request entity too large"}
	}
	return raw, nil
}

// emptyToNull is the bodyparser's JSON reviver: every nested "" becomes null.
func emptyToNull(v js.Value) js.Value {
	switch v.Kind {
	case 's':
		if v.String == "" {
			return js.Value{Kind: 'n'}
		}
	case 'a':
		for i := range v.Items {
			v.Items[i] = emptyToNull(v.Items[i])
		}
	case 'o':
		for k := range v.Fields {
			v.Fields[k] = emptyToNull(v.Fields[k])
		}
	}
	return v
}

// Content-Disposition parameters as @poppinss/multiparty reads them.
var (
	filenameParam = regexp.MustCompile("(?i)\\bfilename=(?:\"(.*?)\"|([!#$%&'*+.0-9A-Z^_`a-z|~-]+))($|; )")
	filenameStar  = regexp.MustCompile(`(?i)\bfilename\*=utf-8''(.*?)($|; )`)
	nameParam     = regexp.MustCompile("(?i)\\bname=(?:\"([^\"]+)\"|([!#$%&'*+.0-9A-Z^_`a-z|~-]+))")
	htmlCharCode  = regexp.MustCompile(`&#([\d]{4});`)
	quotedEscapes = regexp.MustCompile(`%22|\\"`)
)

// partFilename is multiparty's parseFilename; ok=false means no filename
// (the part is a field).
func partFilename(header string) (string, bool) {
	var name string
	if m := filenameParam.FindStringSubmatch(header); m != nil {
		name = m[1] + m[2]
	} else if m := filenameStar.FindStringSubmatch(header); m != nil {
		decoded, err := url.PathUnescape(m[1])
		if err != nil {
			decoded = m[1]
		}
		name = decoded
	} else {
		return "", false
	}
	name = quotedEscapes.ReplaceAllString(name, `"`)
	name = htmlCharCode.ReplaceAllStringFunc(name, func(ref string) string {
		code, _ := strconv.Atoi(ref[2:6])
		return string(rune(code))
	})
	return name[strings.LastIndex(name, `\`)+1:], true
}

type rawPart struct {
	headers map[string]string
	data    []byte
}

// splitMultipart splits a multipart body on its boundary. Header names are
// lower-cased; values keep everything after the colon and leading spaces.
func splitMultipart(body []byte, boundary string) ([]rawPart, error) {
	delimiter := []byte("--" + boundary)
	start := bytes.Index(body, delimiter)
	if start < 0 {
		return nil, errors.New("Part terminated early due to unexpected end of multipart data")
	}
	rest := body[start+len(delimiter):]
	var parts []rawPart
	for {
		if bytes.HasPrefix(rest, []byte("--")) {
			return parts, nil
		}
		rest = bytes.TrimPrefix(rest, []byte("\r\n"))
		headerEnd := bytes.Index(rest, []byte("\r\n\r\n"))
		if headerEnd < 0 {
			return nil, errors.New("Part terminated early due to unexpected end of multipart data")
		}
		headers := map[string]string{}
		for _, line := range strings.Split(string(rest[:headerEnd]), "\r\n") {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			headers[strings.ToLower(key)] = strings.TrimLeft(value, " \t")
		}
		rest = rest[headerEnd+4:]
		end := bytes.Index(rest, append([]byte("\r\n"), delimiter...))
		if end < 0 {
			return nil, errors.New("Part terminated early due to unexpected end of multipart data")
		}
		parts = append(parts, rawPart{headers, rest[:end]})
		rest = rest[end+2+len(delimiter):]
	}
}

// parseMultipart is the bodyparser's multipart handling with autoProcess:
// fields (empty strings become null) and files with their detected type.
// Files appear as Kind 'F' values so request.validateUsing sees both.
func parseMultipart(r *http.Request) (js.Value, error) {
	out := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || params["boundary"] == "" {
		return out, &bodyError{http.StatusBadRequest, "Exception", "Invalid multipart request"}
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, multipartLimit))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return out, &bodyError{http.StatusRequestEntityTooLarge, "Exception", "request entity too large"}
	}
	if err != nil {
		return out, &bodyError{http.StatusBadRequest, "Exception", err.Error()}
	}
	parts, err := splitMultipart(body, params["boundary"])
	if err != nil {
		return out, &bodyError{http.StatusBadRequest, "Exception", err.Error()}
	}
	for _, part := range parts {
		disposition := part.headers["content-disposition"]
		name := ""
		if m := nameParam.FindStringSubmatch(disposition); m != nil {
			name = m[1] + m[2]
		}
		filename, isFile := partFilename(disposition)
		if !isFile {
			var item js.Value
			if len(part.data) == 0 {
				item = js.Value{Kind: 'n'}
			} else {
				item = js.Value{Kind: 's', String: string(part.data)}
			}
			put(&out, name, item)
			continue
		}
		upload := &vine.Upload{FieldName: name, ClientName: filename, ContentType: part.headers["content-type"], Data: part.data}
		if t := filetype.ForUpload(upload.ClientName, upload.ContentType, part.data); t != nil {
			upload.Extname, upload.Type, upload.Subtype, upload.HasExtname = t.Ext, t.Type, t.Subtype, true
		}
		put(&out, name, js.Value{Kind: 'F', File: upload})
	}
	out.Keys = js.ObjectKeyOrder(out.Keys)
	return out, nil
}
