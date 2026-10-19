package forms

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"kaderisasi/web/internal/js"
)

// TokenHash is tokenHash: SHA-256 hex.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// localeCompare is String.prototype.localeCompare in Node's default (ICU,
// en-US) collation.
var localeCompare = collate.New(language.AmericanEnglish)

func stable(v js.Value) js.Value {
	switch v.Kind {
	case 'a':
		out := js.Value{Kind: 'a', Items: make([]js.Value, len(v.Items))}
		for i, item := range v.Items {
			out.Items[i] = stable(item)
		}
		return out
	case 'o':
		keys := append([]string(nil), v.Keys...)
		sort.SliceStable(keys, func(i, j int) bool { return localeCompare.CompareString(keys[i], keys[j]) < 0 })
		out := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
		for _, k := range keys {
			out.Keys = append(out.Keys, k)
			out.Fields[k] = stable(v.Fields[k])
		}
		out.Keys = js.ObjectKeyOrder(out.Keys) // Object.fromEntries enumerates integer keys first
		return out
	}
	return v
}

// SchemaHash is schemaHash: SHA-256 of the JSON with keys in locale order.
func SchemaHash(schema js.Value) string {
	raw, _ := stable(schema).MarshalJSON()
	return TokenHash(string(raw))
}
