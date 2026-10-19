// Package richtext sanitizes administrator-authored HTML exactly like
// kaderisasi-web-be/app/services/rich_text_service.ts (sanitize-html 2.17.6
// on htmlparser2 12 with web-be's allowlist). Interop tests compare the
// output byte for byte with the Node library.
package richtext

import (
	"regexp"
	"strings"

	"kaderisasi/web/internal/js"
)

var allowedTags = set("p", "br", "strong", "b", "em", "i", "u", "s", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li",
	"blockquote", "pre", "code", "a", "hr")

var allowedAttributes = map[string]map[string]bool{
	"a": set("href", "title"), "p": set("style"), "h1": set("style"), "h2": set("style"), "h3": set("style"),
	"h4": set("style"), "h5": set("style"), "h6": set("style"),
}

// sanitize-html defaults that web-be does not override.
var (
	selfClosing          = set("img", "br", "hr", "area", "base", "basefont", "input", "link", "meta", "col")
	nonTextTags          = set("script", "style", "textarea", "option", "xmp")
	nonBooleanAttributes = set("abbr", "accept", "accept-charset", "accesskey", "action", "allow", "alt", "as", "autocapitalize",
		"autocomplete", "blocking", "charset", "cite", "class", "color", "cols", "colspan", "content", "contenteditable", "coords",
		"crossorigin", "data", "datetime", "decoding", "dir", "dirname", "download", "draggable", "enctype", "enterkeyhint",
		"fetchpriority", "for", "form", "formaction", "formenctype", "formmethod", "formtarget", "headers", "height", "hidden",
		"high", "href", "hreflang", "http-equiv", "id", "imagesizes", "imagesrcset", "inputmode", "integrity", "is", "itemid",
		"itemprop", "itemref", "itemtype", "kind", "label", "lang", "list", "loading", "low", "max", "maxlength", "media", "method",
		"min", "minlength", "name", "nonce", "optimum", "pattern", "ping", "placeholder", "popover", "popovertarget",
		"popovertargetaction", "poster", "preload", "referrerpolicy", "rel", "rows", "rowspan", "sandbox", "scope", "shape", "size",
		"sizes", "slot", "span", "spellcheck", "src", "srcdoc", "srclang", "srcset", "start", "step", "style", "tabindex", "target",
		"title", "translate", "type", "usemap", "value", "width", "wrap", "onauxclick", "onafterprint", "onbeforematch",
		"onbeforeprint", "onbeforeunload", "onbeforetoggle", "onblur", "oncancel", "oncanplay", "oncanplaythrough", "onchange",
		"onclick", "onclose", "oncontextlost", "oncontextmenu", "oncontextrestored", "oncopy", "oncuechange", "oncut", "ondblclick",
		"ondrag", "ondragend", "ondragenter", "ondragleave", "ondragover", "ondragstart", "ondrop", "ondurationchange", "onemptied",
		"onended", "onerror", "onfocus", "onformdata", "onhashchange", "oninput", "oninvalid", "onkeydown", "onkeypress", "onkeyup",
		"onlanguagechange", "onload", "onloadeddata", "onloadedmetadata", "onloadstart", "onmessage", "onmessageerror",
		"onmousedown", "onmouseenter", "onmouseleave", "onmousemove", "onmouseout", "onmouseover", "onmouseup", "onoffline",
		"ononline", "onpagehide", "onpageshow", "onpaste", "onpause", "onplay", "onplaying", "onpopstate", "onprogress",
		"onratechange", "onreset", "onresize", "onrejectionhandled", "onscroll", "onscrollend", "onsecuritypolicyviolation",
		"onseeked", "onseeking", "onselect", "onslotchange", "onstalled", "onstorage", "onsubmit", "onsuspend", "ontimeupdate",
		"ontoggle", "onunhandledrejection", "onunload", "onvolumechange", "onwaiting", "onwheel")
	allowedEmptyAttributes = set("alt")
)

const nestingLimit = 20

var validAttributeName = regexp.MustCompile(`^[^\x00\t\n\f\r /<=>]+$`)

type frame struct{ tag string }

type sanitizer struct {
	result        strings.Builder
	stack         []frame
	depth         int
	skipMap       map[int]bool
	skipText      bool
	skipTextDepth int
}

// Sanitize is sanitizeRichText.
func Sanitize(html string) string {
	s := &sanitizer{skipMap: map[int]bool{}}
	parse(html, s)
	return s.result.String()
}

func (s *sanitizer) onopentag(name string, attribs []attribute) {
	if s.skipText {
		s.skipTextDepth++
		return
	}
	s.stack = append(s.stack, frame{tag: name})
	skip := false
	if !allowedTags[name] || s.depth >= nestingLimit {
		skip = true
		s.skipMap[s.depth] = true
		if nonTextTags[name] {
			s.skipText = true
			s.skipTextDepth = 1
		}
	}
	s.depth++
	if skip {
		return
	}
	s.result.WriteString("<" + name)
	if allowed, ok := allowedAttributes[name]; ok {
		for _, a := range jsAttributeOrder(attribs) {
			if !validAttributeName.MatchString(a.name) {
				continue
			}
			value := a.value
			if value == "" && !allowedEmptyAttributes[a.name] && nonBooleanAttributes[a.name] {
				continue
			}
			if !allowed[a.name] {
				continue
			}
			if a.name == "href" && naughtyHref(value) {
				continue
			}
			if a.name == "style" {
				filtered, ok := filterStyle(name, value)
				if !ok || filtered == "" {
					continue
				}
				value = filtered
			}
			s.result.WriteString(" " + a.name)
			if value != "" {
				s.result.WriteString(`="` + escapeHTML(value, true) + `"`)
			} else if allowedEmptyAttributes[a.name] {
				s.result.WriteString(`=""`)
			}
		}
	}
	if selfClosing[name] {
		s.result.WriteString(" />")
	} else {
		s.result.WriteString(">")
	}
}

func (s *sanitizer) ontext(text string) {
	if s.skipText {
		return
	}
	// Every allowed tag is a plain text container (no script, style,
	// textarea or xmp), so text is always escaped.
	s.result.WriteString(escapeHTML(text, false))
}

func (s *sanitizer) onclosetag(name string, _ bool) {
	if s.skipText {
		s.skipTextDepth--
		if s.skipTextDepth != 0 {
			return
		}
		s.skipText = false
	}
	if len(s.stack) == 0 {
		return
	}
	top := s.stack[len(s.stack)-1]
	if top.tag != name {
		return
	}
	s.stack = s.stack[:len(s.stack)-1]
	s.depth--
	if s.skipMap[s.depth] {
		delete(s.skipMap, s.depth)
		return
	}
	if selfClosing[name] {
		return
	}
	s.result.WriteString("</" + name + ">")
}

// jsAttributeOrder is the enumeration order of htmlparser2's attribs object:
// integer-like names first, ascending, then insertion order.
func jsAttributeOrder(attribs []attribute) []attribute {
	names := make([]string, len(attribs))
	byName := map[string]attribute{}
	for i, a := range attribs {
		names[i] = a.name
		byName[a.name] = a
	}
	out := make([]attribute, 0, len(attribs))
	for _, n := range js.ObjectKeyOrder(names) {
		out = append(out, byName[n])
	}
	return out
}

// escapeHTML is sanitize-html's escapeHtml with decodeEntities: true.
func escapeHTML(s string, quote bool) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	if quote {
		s = strings.ReplaceAll(s, `"`, "&quot;")
	}
	return s
}

var (
	controlAndSpace  = regexp.MustCompile(`[\x00-\x20]+`)
	schemePattern    = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9.\-+]*):`)
	protocolRelative = regexp.MustCompile(`^[/\\]{2}`)
	allowedSchemes   = set("http", "https", "mailto")
)

// naughtyHref is launder 1.7's naughtyHref with web-be's schemes and
// allowProtocolRelative: false.
func naughtyHref(href string) bool {
	href = controlAndSpace.ReplaceAllString(href, "")
	for {
		first := strings.Index(href, "<!--")
		if first == -1 {
			break
		}
		last := strings.Index(href[first+4:], "-->")
		if last == -1 {
			break
		}
		href = href[:first] + href[first+4+last+3:]
	}
	m := schemePattern.FindStringSubmatch(href)
	if m == nil {
		return protocolRelative.MatchString(href)
	}
	return !allowedSchemes[strings.ToLower(m[1])]
}
