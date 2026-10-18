package vine

import (
	"net/url"
	"strings"
	"unicode/utf16"
)

// IsEmail ports validator.js 13.15 `isEmail` with the default options Vine
// uses (UTF-8 local part allowed, TLD required, no display name, no IP
// domain). The JavaScript regular expressions have no `u` flag, so character
// classes are evaluated on UTF-16 code units, and `str.length` counts them.
func IsEmail(s string) bool {
	units := utf16.Encode([]rune(s))
	if len(units) > 254 {
		return false
	}
	at := strings.LastIndex(s, "@")
	if at < 0 {
		// "".split('@') yields one part: the whole string is the domain and the
		// user is empty, which can never pass the user-part checks below.
		return false
	}
	domain, user := s[at+1:], s[:at]
	if byteLength(user) > 64 || byteLength(domain) > 254 {
		return false
	}
	if !isFQDN(domain) {
		return false
	}
	if strings.HasPrefix(user, `"`) && strings.HasSuffix(user, `"`) {
		u := utf16.Encode([]rune(user))
		if len(u) < 2 {
			// A lone `"`: JavaScript's slice(1, 0) is empty, which matches.
			return true
		}
		return quotedUserUTF8(u[1 : len(u)-1])
	}
	for _, part := range strings.Split(user, ".") {
		u := utf16.Encode([]rune(part))
		if len(u) == 0 {
			return false
		}
		for _, c := range u {
			if !userUnit(c) {
				return false
			}
		}
	}
	return true
}

// byteLength is validator's isByteLength: encodeURI(str).split(/%..|./).length - 1.
func byteLength(s string) int {
	n := 0
	for _, r := range s {
		if r < 0x80 {
			n++
		} else {
			n += len(url.PathEscape(string(r))) / 3
		}
	}
	return n
}

func asciiAlnum(c uint16) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// emailUserUtf8Part: [a-z\d!#\$%&'\*\+\-\/=\?\^_`{\|}~¡-퟿豈-﷏ﷰ-￯] with /i.
func userUnit(c uint16) bool {
	if asciiAlnum(c) || strings.ContainsRune("!#$%&'*+-/=?^_`{|}~", rune(c)) {
		return true
	}
	return utf8Range(c, 0x00A1)
}

func utf8Range(c, low uint16) bool {
	return (c >= low && c <= 0xD7FF) || (c >= 0xF900 && c <= 0xFDCF) || (c >= 0xFDF0 && c <= 0xFFEF)
}

// jsWhitespace is the set matched by JavaScript's \s.
func jsWhitespace(c uint16) bool {
	switch c {
	case 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x20, 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return c >= 0x2000 && c <= 0x200A
}

// quotedEmailUserUtf8:
// ^([\s\x01-\x08\x0b\x0c\x0e-\x1f\x7f\x21\x23-\x5b\x5d-\x7e -퟿豈-﷏ﷰ-￯]|(\\[\x01-\x09\x0b\x0c\x0d-\x7f -퟿豈-﷏ﷰ-￯]))*$
func quotedUserUTF8(u []uint16) bool {
	plain := func(c uint16) bool {
		return jsWhitespace(c) || (c >= 0x01 && c <= 0x08) || c == 0x0b || c == 0x0c || (c >= 0x0e && c <= 0x1f) ||
			c == 0x7f || c == 0x21 || (c >= 0x23 && c <= 0x5b) || (c >= 0x5d && c <= 0x7e) || utf8Range(c, 0x00A0)
	}
	escaped := func(c uint16) bool {
		return (c >= 0x01 && c <= 0x09) || c == 0x0b || c == 0x0c || (c >= 0x0d && c <= 0x7f) || utf8Range(c, 0x00A0)
	}
	// A backslash (0x5c) is never a plain unit, so it must start an escape;
	// matching left to right is therefore exact.
	for i := 0; i < len(u); i++ {
		if plain(u[i]) {
			continue
		}
		if u[i] == '\\' && i+1 < len(u) && escaped(u[i+1]) {
			i++
			continue
		}
		return false
	}
	return true
}

// isFQDN ports validator's isFQDN with require_tld and every other option off.
func isFQDN(domain string) bool {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}
	tld := utf16.Encode([]rune(parts[len(parts)-1]))
	if !tldValid(tld) {
		return false
	}
	for _, c := range tld {
		if jsWhitespace(c) {
			return false
		}
	}
	allDigits := len(tld) > 0
	for _, c := range tld {
		allDigits = allDigits && c >= '0' && c <= '9'
	}
	if allDigits {
		return false
	}
	for _, p := range parts {
		u := utf16.Encode([]rune(p))
		if len(u) > 63 || len(u) == 0 {
			return false
		}
		for _, c := range u {
			// [a-z_¡-￿0-9-] with /i, then no full-width ASCII and no underscore.
			if !(asciiAlnum(c) || c == '_' || c == '-' || c >= 0x00a1) || (c >= 0xff01 && c <= 0xff5e) || c == '_' {
				return false
			}
		}
		if u[0] == '-' || u[len(u)-1] == '-' {
			return false
		}
	}
	return true
}

// tld: ^([a-z¡-¨ª-퟿豈-﷏ﷰ-￯]{2,}|xn[a-z0-9-]{2,})$ with /i.
func tldValid(u []uint16) bool {
	letters := len(u) >= 2
	for _, c := range u {
		letters = letters && ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= 0xA1 && c <= 0xA8) || (c >= 0xAA && c <= 0xD7FF) || (c >= 0xF900 && c <= 0xFDCF) || (c >= 0xFDF0 && c <= 0xFFEF))
	}
	if letters {
		return true
	}
	if len(u) >= 4 && (u[0] == 'x' || u[0] == 'X') && (u[1] == 'n' || u[1] == 'N') {
		for _, c := range u[2:] {
			if !(asciiAlnum(c) || c == '-') {
				return false
			}
		}
		return true
	}
	return false
}
