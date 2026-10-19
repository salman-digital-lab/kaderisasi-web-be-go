package richtext

import (
	"strings"
	"sync"
)

type decodeMode int

const (
	modeLegacy    decodeMode = iota // text: legacy references without ';' decode
	modeAttribute                   // attribute: not before '=' or an alphanumeric
)

type entityDecoder struct{ mode decodeMode }

// entityIndex holds every name prefix (without ';') so the walk can stop
// exactly where the entities package's trie stops.
var entityIndex = sync.OnceValue(func() map[string]bool {
	entity, entity2 := entityMaps()
	prefixes := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSuffix(name, ";")
		for i := 1; i <= len(name); i++ {
			prefixes[name[:i]] = true
		}
	}
	for name := range entity {
		add(name)
	}
	for name := range entity2 {
		add(name)
	}
	return prefixes
})

func lookup(name string) (string, bool) {
	entity, entity2 := entityMaps()
	if r, ok := entity[name]; ok {
		return string(r), true
	}
	if rs, ok := entity2[name]; ok {
		return string(rs[0]) + string(rs[1]), true
	}
	return "", false
}

func isAlnum(c byte) bool { return isASCIIAlpha(c) || (c >= '0' && c <= '9') }

// c1Replacements is entities' decodeMap for numeric references.
var c1Replacements = map[int]rune{0: 0xFFFD, 128: 8364, 130: 8218, 131: 402, 132: 8222, 133: 8230, 134: 8224, 135: 8225, 136: 710, 137: 8240,
	138: 352, 139: 8249, 140: 338, 142: 381, 145: 8216, 146: 8217, 147: 8220, 148: 8221, 149: 8226, 150: 8211, 151: 8212, 152: 732,
	153: 8482, 154: 353, 155: 8250, 156: 339, 158: 382, 159: 376}

func replaceCodePoint(cp int) rune {
	if (cp >= 0xD800 && cp <= 0xDFFF) || cp > 0x10FFFF {
		return 0xFFFD
	}
	if r, ok := c1Replacements[cp]; ok {
		return r
	}
	return rune(cp)
}

// decodeReference decodes the character reference starting with '&' at amp.
// It returns the decoded text and how many bytes it consumed, or 0 when the
// '&' is literal text.
func decodeReference(buf string, amp int, mode decodeMode) (string, int) {
	i := amp + 1
	if i < len(buf) && buf[i] == '#' {
		return decodeNumeric(buf, amp)
	}
	index := entityIndex()
	lastLegacy := 0 // length of the longest name usable without ';'
	length := 0
	for {
		name := buf[amp+1 : amp+1+length]
		if i >= len(buf) {
			if lastLegacy > 0 && (mode != modeAttribute || lastLegacy == length) {
				text, _ := lookup(buf[amp+1 : amp+1+lastLegacy])
				return text, 1 + lastLegacy
			}
			return "", 0
		}
		c := buf[i]
		if c == ';' && length > 0 {
			if text, ok := lookup(name + ";"); ok {
				return text, length + 2
			}
		}
		if !isAlnum(c) || !index[name+string(c)] {
			if lastLegacy == 0 {
				return "", 0
			}
			if mode == modeAttribute {
				_, legacy := lookup(name)
				_, terminated := lookup(name + ";")
				if !(legacy || terminated) || c == '=' || isAlnum(c) {
					return "", 0
				}
			}
			text, _ := lookup(buf[amp+1 : amp+1+lastLegacy])
			return text, 1 + lastLegacy
		}
		length++
		i++
		if _, ok := lookup(buf[amp+1 : amp+1+length]); ok {
			lastLegacy = length
		}
	}
}

func decodeNumeric(buf string, amp int) (string, int) {
	i := amp + 2
	consumed := 2
	expected := 2
	hex := false
	if i < len(buf) && buf[i]|0x20 == 'x' {
		hex = true
		consumed, expected = 3, 3
		i++
	}
	if i >= len(buf) && !hex {
		return "", 0 // "&#" at the end of the input
	}
	result := 0
	for ; i < len(buf); i++ {
		c := buf[i]
		digit := -1
		switch {
		case c >= '0' && c <= '9':
			digit = int(c - '0')
		case hex && c|0x20 >= 'a' && c|0x20 <= 'f':
			digit = int(c|0x20-'a') + 10
		}
		if digit < 0 {
			break
		}
		if result <= 0x10FFFF {
			if hex {
				result = result*16 + digit
			} else {
				result = result*10 + digit
			}
		}
		consumed++
	}
	if consumed <= expected {
		return "", 0
	}
	if i < len(buf) && buf[i] == ';' {
		consumed++
	}
	return string(replaceCodePoint(result)), consumed
}
