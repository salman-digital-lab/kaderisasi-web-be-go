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

// all is Adonis's request.all(): the query string merged with the parsed
// body, body keys winning. The bodyparser turns empty body strings into null
// (convertEmptyStringsToNull); query string values are kept as they are.
func all(r *http.Request) (vine.Input, error) {
	input := vine.Input{}
	for key, values := range r.URL.Query() {
		input[key] = queryValue(values)
	}
	body, err := parseBody(r)
	if err != nil {
		return nil, err
	}
	for key, value := range body {
		input[key] = value
	}
	return input, nil
}

func queryValue(values []string) json.RawMessage {
	var raw []byte
	if len(values) == 1 {
		raw, _ = json.Marshal(values[0])
	} else {
		raw, _ = json.Marshal(values)
	}
	return raw
}

func parseBody(r *http.Request) (vine.Input, error) {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nil, nil
	}
	if r.Body == nil || (r.ContentLength == 0 && len(r.TransferEncoding) == 0) {
		return nil, nil
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch mediaType {
	case "application/json", "application/json-patch+json", "application/vnd.api+json", "application/csp-report":
		raw, err := readLimited(r.Body, jsonLimit)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return nil, nil
		}
		if !strictJSON.Match(raw) {
			return nil, &bodyError{http.StatusUnprocessableEntity, "Exception", "Invalid JSON, only supports object and array"}
		}
		var parsed any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&parsed); err != nil || decoder.More() {
			// JSON.parse's SyntaxError; V8's wording is only reproduced for a
			// truncated document.
			text := "Invalid JSON body"
			if errors.Is(err, io.ErrUnexpectedEOF) {
				text = "Unexpected end of JSON input"
			}
			return nil, &bodyError{http.StatusBadRequest, "SyntaxError", text}
		}
		return objectInput(emptyToNull(parsed)), nil
	case "application/x-www-form-urlencoded":
		raw, err := readLimited(r.Body, formLimit)
		if err != nil {
			return nil, err
		}
		values, err := url.ParseQuery(string(raw))
		if err != nil {
			return nil, &bodyError{http.StatusBadRequest, "Error", "Invalid form body"}
		}
		input := vine.Input{}
		for key, list := range values {
			var value any = list
			if len(list) == 1 {
				value = list[0]
			}
			input[key], _ = json.Marshal(emptyToNull(value))
		}
		return input, nil
	}
	return nil, nil
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
func emptyToNull(v any) any {
	switch value := v.(type) {
	case string:
		if value == "" {
			return nil
		}
	case []any:
		for i := range value {
			value[i] = emptyToNull(value[i])
		}
	case []string:
		out := make([]any, len(value))
		for i := range value {
			out[i] = emptyToNull(value[i])
		}
		return out
	case map[string]any:
		for k := range value {
			value[k] = emptyToNull(value[k])
		}
	}
	return v
}

// objectInput spreads a parsed body into request.all(); an array body
// contributes its indexes as keys, like JavaScript object spread.
func objectInput(v any) vine.Input {
	input := vine.Input{}
	switch value := v.(type) {
	case map[string]any:
		for k, item := range value {
			input[k], _ = json.Marshal(item)
		}
	case []any:
		for i, item := range value {
			raw, _ := json.Marshal(item)
			key, _ := json.Marshal(i)
			input[string(key)] = raw
		}
	}
	return input
}
