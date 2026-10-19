package richtext

import "strings"

// This file ports htmlparser2 12's Parser in HTML mode (lower-cased tag and
// attribute names, no self-closing recognition outside foreign content).

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

var (
	formTags         = set("input", "option", "optgroup", "select", "button", "datalist", "textarea")
	pTag             = set("p")
	headingTags      = set("h1", "h2", "h3", "h4", "h5", "h6", "p")
	tableSectionTags = set("thead", "tbody")
	ddtTags          = set("dd", "dt")
	rtpTags          = set("rt", "rp")
	openImpliesClose = map[string]map[string]bool{
		"tr": set("tr", "th", "td"), "th": set("th"), "td": set("thead", "th", "td"), "body": set("head", "link", "script"),
		"a": set("a"), "li": set("li"), "p": pTag, "h1": headingTags, "h2": headingTags, "h3": headingTags, "h4": headingTags,
		"h5": headingTags, "h6": headingTags, "select": formTags, "input": formTags, "output": formTags, "button": formTags,
		"datalist": formTags, "textarea": formTags, "option": set("option"), "optgroup": set("optgroup", "option"),
		"dd": ddtTags, "dt": ddtTags, "address": pTag, "article": pTag, "aside": pTag, "blockquote": pTag, "details": pTag,
		"div": pTag, "dl": pTag, "fieldset": pTag, "figcaption": pTag, "figure": pTag, "footer": pTag, "form": pTag,
		"header": pTag, "hr": pTag, "main": pTag, "nav": pTag, "ol": pTag, "pre": pTag, "section": pTag, "table": pTag,
		"ul": pTag, "rt": rtpTags, "rp": rtpTags, "tbody": tableSectionTags, "tfoot": tableSectionTags,
	}
	voidElements = set("area", "base", "basefont", "br", "col", "command", "embed", "frame", "hr", "img", "input", "isindex",
		"keygen", "link", "meta", "param", "source", "track", "wbr")
	foreignContextElements  = set("math", "svg")
	htmlIntegrationElements = set("mi", "mo", "mn", "ms", "mtext", "annotation-xml", "foreignObject", "desc", "title")
	svgTagNameAdjustments   = map[string]string{
		"altglyph": "altGlyph", "altglyphdef": "altGlyphDef", "altglyphitem": "altGlyphItem", "animatecolor": "animateColor",
		"animatemotion": "animateMotion", "animatetransform": "animateTransform", "clippath": "clipPath", "feblend": "feBlend",
		"fecolormatrix": "feColorMatrix", "fecomponenttransfer": "feComponentTransfer", "fecomposite": "feComposite",
		"feconvolvematrix": "feConvolveMatrix", "fediffuselighting": "feDiffuseLighting", "fedisplacementmap": "feDisplacementMap",
		"fedistantlight": "feDistantLight", "fedropshadow": "feDropShadow", "feflood": "feFlood", "fefunca": "feFuncA",
		"fefuncb": "feFuncB", "fefuncg": "feFuncG", "fefuncr": "feFuncR", "fegaussianblur": "feGaussianBlur", "feimage": "feImage",
		"femerge": "feMerge", "femergenode": "feMergeNode", "femorphology": "feMorphology", "feoffset": "feOffset",
		"fepointlight": "fePointLight", "fespecularlighting": "feSpecularLighting", "fespotlight": "feSpotLight",
		"fetile": "feTile", "feturbulence": "feTurbulence", "foreignobject": "foreignObject", "glyphref": "glyphRef",
		"lineargradient": "linearGradient", "radialgradient": "radialGradient", "textpath": "textPath",
	}
)

type foreign int

const (
	foreignNone foreign = iota
	foreignSvg
	foreignMath
)

// attribute keeps insertion order; the first occurrence of a name wins.
type attribute struct{ name, value string }

// handler receives htmlparser2's onopentag / ontext / onclosetag events.
type handler interface {
	onopentag(name string, attribs []attribute)
	ontext(text string)
	onclosetag(name string, implied bool)
}

type parser struct {
	buf        string
	h          handler
	tagname    string
	attribname string
	attribval  strings.Builder
	attribs    []attribute
	hasAttribs bool
	stack      []string // stack[0] is the innermost element
	foreign    []foreign
}

func parse(input string, h handler) {
	p := &parser{buf: input, h: h, foreign: []foreign{foreignNone}}
	newTokenizer(p, input).run()
}

// slice is String.prototype.slice, including negative indexes: htmlparser2
// emits trailing data of an unfinished `<tag/` from sectionStart -1, which
// JavaScript reads as the last character.
func (p *parser) slice(start, end int) string {
	if start == end {
		return ""
	}
	clamp := func(i int) int {
		if i < 0 {
			i += len(p.buf)
			if i < 0 {
				i = 0
			}
		}
		if i > len(p.buf) {
			i = len(p.buf)
		}
		return i
	}
	start, end = clamp(start), clamp(end)
	if start >= end {
		return ""
	}
	return p.buf[start:end]
}

func (p *parser) ontext(start, end int)            { p.h.ontext(p.slice(start, end)) }
func (p *parser) ontextentity(text string, _ int)  { p.h.ontext(text) }
func (p *parser) isInForeignContext() bool         { return p.foreign[0] != foreignNone }
func (p *parser) isVoidElement(name string) bool   { return voidElements[name] }
func (p *parser) onattribdata(start, end int)      { p.attribval.WriteString(p.slice(start, end)) }
func (p *parser) onattribentity(text string)       { p.attribval.WriteString(text) }
func (p *parser) ondeclaration(int, int)           {}
func (p *parser) onprocessinginstruction(int, int) {}
func (p *parser) oncomment(int, int, int)          {}

func (p *parser) oncdata(start, end, offset int) {
	if p.isInForeignContext() {
		p.h.ontext(p.slice(start, end-offset))
	}
}

func (p *parser) readTagName(start, end int) string {
	name := strings.ToLower(p.slice(start, end))
	if p.foreign[0] == foreignSvg {
		if adjusted, ok := svgTagNameAdjustments[name]; ok {
			return adjusted
		}
		return name
	}
	if len(p.foreign) > 1 {
		if adjusted, ok := svgTagNameAdjustments[name]; ok {
			for _, open := range p.stack {
				if open == adjusted {
					return adjusted
				}
			}
		}
	}
	if !p.isInForeignContext() && name == "image" {
		return "img"
	}
	return name
}

func (p *parser) onopentagname(start, end int) { p.emitOpenTag(p.readTagName(start, end)) }

func (p *parser) emitOpenTag(name string) {
	p.tagname = name
	if name == "form" {
		for _, open := range p.stack {
			if open == "form" {
				p.tagname = ""
				return
			}
		}
	}
	if implies := openImpliesClose[name]; implies != nil {
		for len(p.stack) > 0 && implies[p.stack[0]] {
			p.popElement(true)
		}
	}
	if !p.isVoidElement(name) {
		p.stack = append([]string{name}, p.stack...)
		switch {
		case name == "svg":
			p.foreign = append([]foreign{foreignSvg}, p.foreign...)
		case name == "math":
			p.foreign = append([]foreign{foreignMath}, p.foreign...)
		case htmlIntegrationElements[name]:
			p.foreign = append([]foreign{foreignNone}, p.foreign...)
		}
	}
	p.attribs, p.hasAttribs = nil, true
}

func (p *parser) endOpenTag() {
	if p.hasAttribs {
		p.h.onopentag(p.tagname, p.attribs)
		p.attribs, p.hasAttribs = nil, false
	}
	if p.isVoidElement(p.tagname) {
		p.h.onclosetag(p.tagname, true)
	}
	p.tagname = ""
}

func (p *parser) onopentagend(int) { p.endOpenTag() }

func (p *parser) onclosetag(start, end int) {
	name := p.readTagName(start, end)
	if !p.isVoidElement(name) {
		pos := -1
		for i, open := range p.stack {
			if open == name {
				pos = i
				break
			}
		}
		if pos != -1 {
			for i := 0; i < pos; i++ {
				p.popElement(true)
			}
			p.popElement(false)
		} else if name == "p" {
			p.emitOpenTag("p")
			p.closeCurrentTag(true)
		}
	} else if name == "br" {
		p.h.onopentag("br", nil)
		p.h.onclosetag("br", false)
	}
}

func (p *parser) onselfclosingtag(end int) {
	if p.isInForeignContext() {
		p.closeCurrentTag(false)
	} else {
		p.onopentagend(end)
	}
}

func (p *parser) popElement(implied bool) {
	element := p.stack[0]
	p.stack = p.stack[1:]
	if foreignContextElements[element] || htmlIntegrationElements[element] {
		p.foreign = p.foreign[1:]
	}
	p.h.onclosetag(element, implied)
}

func (p *parser) closeCurrentTag(isOpenImplied bool) {
	name := p.tagname
	p.endOpenTag()
	if len(p.stack) > 0 && p.stack[0] == name {
		p.popElement(!isOpenImplied)
	}
}

func (p *parser) onattribname(start, end int) {
	p.attribname = strings.ToLower(p.slice(start, end))
}

func (p *parser) onattribend(quoteType, int) {
	if p.hasAttribs {
		seen := false
		for _, a := range p.attribs {
			seen = seen || a.name == p.attribname
		}
		if !seen {
			p.attribs = append(p.attribs, attribute{p.attribname, p.attribval.String()})
		}
	}
	p.attribval.Reset()
}

func (p *parser) onend() {
	for _, open := range p.stack {
		p.h.onclosetag(open, true)
	}
}
