package richtext

// This file ports htmlparser2 12's Tokenizer in HTML mode with
// decodeEntities: true, which is how sanitize-html 2.17 drives it. The input
// is scanned byte by byte; every syntax character is ASCII, so UTF-8
// sequences are only ever sliced, never interpreted.

type state int

const (
	sText state = iota + 1
	sBeforeTagName
	sInTagName
	sInSelfClosingTag
	sBeforeClosingTagName
	sInClosingTagName
	sAfterClosingTagName
	sBeforeAttributeName
	sInAttributeName
	sAfterAttributeName
	sBeforeAttributeValue
	sInAttributeValueDq
	sInAttributeValueSq
	sInAttributeValueNq
	sBeforeDeclaration
	sInDeclaration
	sInProcessingInstruction
	sBeforeComment
	sCDATASequence
	sDeclarationSequence
	sInSpecialComment
	sInCommentLike
	sSpecialStartSequence
	sInSpecialTag
	sInPlainText
	sInEntity
)

type quoteType int

const (
	quoteNone quoteType = iota
	quoteUnquoted
	quoteSingle
	quoteDouble
)

var (
	seqEmpty       = ""
	seqCdata       = "CDATA["
	seqCdataEnd    = "]]>"
	seqCommentEnd  = "--!>"
	seqDoctype     = "doctype"
	seqIframeEnd   = "</iframe"
	seqNoembedEnd  = "</noembed"
	seqNoframesEnd = "</noframes"
	seqPlaintext   = "</plaintext"
	seqScriptEnd   = "</script"
	seqStyleEnd    = "</style"
	seqTitleEnd    = "</title"
	seqTextareaEnd = "</textarea"
	seqXmpEnd      = "</xmp"
)

var specialStart = map[byte]string{'i': seqIframeEnd, 'n': seqNoembedEnd, 'p': seqPlaintext, 's': seqScriptEnd, 't': seqTitleEnd, 'x': seqXmpEnd}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\t' || c == '\f' || c == '\r'
}
func isEndOfTagSection(c byte) bool { return c == '/' || c == '>' || isWhitespace(c) }
func isASCIIAlpha(c byte) bool      { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

type callbacks interface {
	ontext(start, end int)
	ontextentity(text string, end int)
	onattribdata(start, end int)
	onattribentity(text string)
	onattribend(q quoteType, end int)
	onattribname(start, end int)
	onopentagname(start, end int)
	onopentagend(end int)
	onselfclosingtag(end int)
	onclosetag(start, end int)
	oncomment(start, end, offset int)
	oncdata(start, end, offset int)
	ondeclaration(start, end int)
	onprocessinginstruction(start, end int)
	onend()
	isInForeignContext() bool
}

type tokenizer struct {
	cbs           callbacks
	state         state
	buf           string
	sectionStart  int
	index         int
	entityStart   int
	baseState     state
	isSpecial     bool
	currentSeq    string
	sequenceIndex int
	entity        entityDecoder
}

func newTokenizer(cbs callbacks, input string) *tokenizer {
	return &tokenizer{cbs: cbs, state: sText, buf: input, baseState: sText}
}

func (t *tokenizer) stateText(c byte) {
	if c == '<' {
		if t.index > t.sectionStart {
			t.cbs.ontext(t.sectionStart, t.index)
		}
		t.state = sBeforeTagName
		t.sectionStart = t.index
	} else if c == '&' {
		t.startEntity()
	}
}

func (t *tokenizer) enterTagBody() {
	if t.currentSeq == seqPlaintext {
		t.currentSeq = seqEmpty
		t.state = sInPlainText
	} else if t.isSpecial {
		t.state = sInSpecialTag
		t.sequenceIndex = 0
	} else {
		t.state = sText
	}
}

func (t *tokenizer) stateSpecialStartSequence(c byte) {
	lower := c | 0x20
	if t.sequenceIndex < len(t.currentSeq) {
		if lower == t.currentSeq[t.sequenceIndex] {
			t.sequenceIndex++
			return
		}
		if t.sequenceIndex == 3 {
			if t.currentSeq == seqScriptEnd && lower == seqStyleEnd[3] {
				t.currentSeq, t.sequenceIndex = seqStyleEnd, 4
				return
			}
			if t.currentSeq == seqTitleEnd && lower == seqTextareaEnd[3] {
				t.currentSeq, t.sequenceIndex = seqTextareaEnd, 4
				return
			}
		} else if t.sequenceIndex == 4 && t.currentSeq == seqNoembedEnd && lower == seqNoframesEnd[4] {
			t.currentSeq, t.sequenceIndex = seqNoframesEnd, 5
			return
		}
	} else if isEndOfTagSection(c) {
		t.sequenceIndex = 0
		t.state = sInTagName
		t.stateInTagName(c)
		return
	}
	t.isSpecial = false
	t.currentSeq = seqEmpty
	t.sequenceIndex = 0
	t.state = sInTagName
	t.stateInTagName(c)
}

func (t *tokenizer) stateCDATASequence(c byte) {
	if c == seqCdata[t.sequenceIndex] {
		t.sequenceIndex++
		if t.sequenceIndex == len(seqCdata) {
			t.state = sInCommentLike
			t.currentSeq = seqCdataEnd
			t.sequenceIndex = 0
			t.sectionStart = t.index + 1
		}
	} else {
		t.sequenceIndex = 0
		t.state = sInSpecialComment
		t.stateInSpecialComment(c)
	}
}

func (t *tokenizer) fastForwardTo(c byte) bool {
	for t.index++; t.index < len(t.buf); t.index++ {
		if t.buf[t.index] == c {
			return true
		}
	}
	t.index = len(t.buf) - 1
	return false
}

func (t *tokenizer) emitComment(offset int) {
	t.cbs.oncomment(t.sectionStart, t.index, offset)
	t.sequenceIndex = 0
	t.sectionStart = t.index + 1
	t.state = sText
}

func (t *tokenizer) stateInCommentLike(c byte) {
	switch {
	case t.currentSeq == seqCommentEnd && t.sequenceIndex <= 1 && t.index == t.sectionStart+t.sequenceIndex && c == '>':
		t.emitComment(t.sequenceIndex)
	case t.currentSeq == seqCommentEnd && t.sequenceIndex == 2 && c == '>':
		t.emitComment(2)
	case t.currentSeq == seqCommentEnd && t.sequenceIndex == len(t.currentSeq)-1 && c != '>':
		if c == '-' {
			t.sequenceIndex = 1
		} else {
			t.sequenceIndex = 0
		}
	case c == t.currentSeq[t.sequenceIndex]:
		t.sequenceIndex++
		if t.sequenceIndex == len(t.currentSeq) {
			if t.currentSeq == seqCdataEnd {
				t.cbs.oncdata(t.sectionStart, t.index, 2)
			} else {
				t.cbs.oncomment(t.sectionStart, t.index, 3)
			}
			t.sequenceIndex = 0
			t.sectionStart = t.index + 1
			t.state = sText
		}
	case t.sequenceIndex == 0:
		if t.fastForwardTo(t.currentSeq[0]) {
			t.sequenceIndex = 1
		}
	case c != t.currentSeq[t.sequenceIndex-1]:
		t.sequenceIndex = 0
	}
}

func (t *tokenizer) stateInSpecialTag(c byte) {
	if t.sequenceIndex == len(t.currentSeq) {
		if isEndOfTagSection(c) {
			endOfText := t.index - len(t.currentSeq)
			if t.sectionStart < endOfText {
				actual := t.index
				t.index = endOfText
				t.cbs.ontext(t.sectionStart, endOfText)
				t.index = actual
			}
			t.isSpecial = false
			t.sectionStart = endOfText + 2
			t.stateInClosingTagName(c)
			return
		}
		t.sequenceIndex = 0
	}
	if c|0x20 == t.currentSeq[t.sequenceIndex] {
		t.sequenceIndex++
	} else if t.sequenceIndex == 0 {
		if t.currentSeq == seqTitleEnd || t.currentSeq == seqTextareaEnd {
			if c == '&' {
				t.startEntity()
			}
		} else if t.fastForwardTo('<') {
			t.sequenceIndex = 1
		}
	} else if c == '<' {
		t.sequenceIndex = 1
	} else {
		t.sequenceIndex = 0
	}
}

func (t *tokenizer) stateBeforeTagName(c byte) {
	switch {
	case c == '!':
		t.state = sBeforeDeclaration
		t.sectionStart = t.index + 1
	case c == '?':
		t.state = sInSpecialComment
		t.sectionStart = t.index
	case isASCIIAlpha(c):
		t.sectionStart = t.index
		special, ok := specialStart[c|0x20]
		if t.cbs.isInForeignContext() || !ok {
			t.state = sInTagName
		} else {
			t.isSpecial = true
			t.currentSeq = special
			t.sequenceIndex = 3
			t.state = sSpecialStartSequence
		}
	case c == '/':
		t.state = sBeforeClosingTagName
	default:
		t.state = sText
		t.stateText(c)
	}
}

func (t *tokenizer) stateInTagName(c byte) {
	if isEndOfTagSection(c) {
		t.cbs.onopentagname(t.sectionStart, t.index)
		t.sectionStart = -1
		t.state = sBeforeAttributeName
		t.stateBeforeAttributeName(c)
	}
}

func (t *tokenizer) stateBeforeClosingTagName(c byte) {
	switch {
	case isWhitespace(c):
		t.state = sInSpecialComment
		t.sectionStart = t.index
	case c == '>':
		t.state = sText
		t.sectionStart = t.index + 1
	default:
		if isASCIIAlpha(c) {
			t.state = sInClosingTagName
		} else {
			t.state = sInSpecialComment
		}
		t.sectionStart = t.index
	}
}

func (t *tokenizer) stateInClosingTagName(c byte) {
	if isEndOfTagSection(c) {
		t.cbs.onclosetag(t.sectionStart, t.index)
		t.sectionStart = -1
		t.state = sAfterClosingTagName
		t.stateAfterClosingTagName(c)
	}
}

func (t *tokenizer) stateAfterClosingTagName(c byte) {
	if c == '>' || t.fastForwardTo('>') {
		t.state = sText
		t.sectionStart = t.index + 1
	}
}

func (t *tokenizer) stateBeforeAttributeName(c byte) {
	switch {
	case c == '>':
		t.cbs.onopentagend(t.index)
		t.enterTagBody()
		t.sectionStart = t.index + 1
	case c == '/':
		t.state = sInSelfClosingTag
	case !isWhitespace(c):
		t.state = sInAttributeName
		t.sectionStart = t.index
	}
}

func (t *tokenizer) stateInSelfClosingTag(c byte) {
	if c == '>' {
		t.cbs.onselfclosingtag(t.index)
		t.sectionStart = t.index + 1
		// recognizeSelfClosing is false in HTML mode.
		t.enterTagBody()
	} else if !isWhitespace(c) {
		t.state = sBeforeAttributeName
		t.stateBeforeAttributeName(c)
	}
}

func (t *tokenizer) stateInAttributeName(c byte) {
	if c == '=' || isEndOfTagSection(c) {
		t.cbs.onattribname(t.sectionStart, t.index)
		t.sectionStart = t.index
		t.state = sAfterAttributeName
		t.stateAfterAttributeName(c)
	}
}

func (t *tokenizer) stateAfterAttributeName(c byte) {
	switch {
	case c == '=':
		t.state = sBeforeAttributeValue
	case c == '/' || c == '>':
		t.cbs.onattribend(quoteNone, t.sectionStart)
		t.sectionStart = -1
		t.state = sBeforeAttributeName
		t.stateBeforeAttributeName(c)
	case !isWhitespace(c):
		t.cbs.onattribend(quoteNone, t.sectionStart)
		t.state = sInAttributeName
		t.sectionStart = t.index
	}
}

func (t *tokenizer) stateBeforeAttributeValue(c byte) {
	switch {
	case c == '"':
		t.state = sInAttributeValueDq
		t.sectionStart = t.index + 1
	case c == '\'':
		t.state = sInAttributeValueSq
		t.sectionStart = t.index + 1
	case !isWhitespace(c):
		t.sectionStart = t.index
		t.state = sInAttributeValueNq
		t.stateInAttributeValueNoQuotes(c)
	}
}

func (t *tokenizer) handleInAttributeValue(c, quote byte) {
	if c == quote {
		t.cbs.onattribdata(t.sectionStart, t.index)
		t.sectionStart = -1
		q := quoteSingle
		if quote == '"' {
			q = quoteDouble
		}
		t.cbs.onattribend(q, t.index+1)
		t.state = sBeforeAttributeName
	} else if c == '&' {
		t.startEntity()
	}
}

func (t *tokenizer) stateInAttributeValueNoQuotes(c byte) {
	if isWhitespace(c) || c == '>' {
		t.cbs.onattribdata(t.sectionStart, t.index)
		t.sectionStart = -1
		t.cbs.onattribend(quoteUnquoted, t.index)
		t.state = sBeforeAttributeName
		t.stateBeforeAttributeName(c)
	} else if c == '&' {
		t.startEntity()
	}
}

func (t *tokenizer) stateBeforeDeclaration(c byte) {
	switch {
	case c == '[':
		t.state = sCDATASequence
		t.sequenceIndex = 0
	case c|0x20 == seqDoctype[0]:
		t.state = sDeclarationSequence
		t.currentSeq = seqDoctype
		t.sequenceIndex = 1
	case c == '>':
		t.cbs.oncomment(t.sectionStart, t.index, 0)
		t.state = sText
		t.sectionStart = t.index + 1
	case c == '-':
		t.state = sBeforeComment
	default:
		t.state = sInSpecialComment
	}
}

func (t *tokenizer) stateDeclarationSequence(c byte) {
	switch {
	case t.sequenceIndex == len(t.currentSeq):
		t.state = sInDeclaration
		t.stateInDeclaration(c)
	case c|0x20 == t.currentSeq[t.sequenceIndex]:
		t.sequenceIndex++
	case c == '>':
		t.cbs.oncomment(t.sectionStart, t.index, 0)
		t.state = sText
		t.sectionStart = t.index + 1
	default:
		t.state = sInSpecialComment
	}
}

func (t *tokenizer) stateInDeclaration(c byte) {
	if c == '>' || t.fastForwardTo('>') {
		t.cbs.ondeclaration(t.sectionStart, t.index)
		t.state = sText
		t.sectionStart = t.index + 1
	}
}

func (t *tokenizer) stateBeforeComment(c byte) {
	switch {
	case c == '-':
		t.state = sInCommentLike
		t.currentSeq = seqCommentEnd
		t.sequenceIndex = 0
		t.sectionStart = t.index + 1
	case c == '>':
		t.cbs.oncomment(t.sectionStart, t.index, 0)
		t.state = sText
		t.sectionStart = t.index + 1
	default:
		t.state = sInSpecialComment
	}
}

func (t *tokenizer) stateInSpecialComment(c byte) {
	if c == '>' || t.fastForwardTo('>') {
		t.cbs.oncomment(t.sectionStart, t.index, 0)
		t.state = sText
		t.sectionStart = t.index + 1
	}
}

func (t *tokenizer) startEntity() {
	t.baseState = t.state
	t.state = sInEntity
	t.entityStart = t.index
	mode := modeAttribute
	if t.baseState == sText || t.baseState == sInSpecialTag {
		mode = modeLegacy
	}
	t.entity = entityDecoder{mode: mode}
}

// stateInEntity decodes the whole reference at once (the input is complete,
// so the streaming decoder's "incomplete" result never happens mid-buffer).
func (t *tokenizer) stateInEntity() {
	text, consumed := decodeReference(t.buf, t.entityStart, t.entity.mode)
	t.state = t.baseState
	if consumed == 0 {
		t.index--
		return
	}
	t.emitCodePoint(text, consumed)
}

func (t *tokenizer) emitCodePoint(text string, consumed int) {
	if t.baseState != sText && t.baseState != sInSpecialTag {
		if t.sectionStart < t.entityStart {
			t.cbs.onattribdata(t.sectionStart, t.entityStart)
		}
		t.sectionStart = t.entityStart + consumed
		t.index = t.sectionStart - 1
		t.cbs.onattribentity(text)
	} else {
		if t.sectionStart < t.entityStart {
			t.cbs.ontext(t.sectionStart, t.entityStart)
		}
		t.sectionStart = t.entityStart + consumed
		t.index = t.sectionStart - 1
		t.cbs.ontextentity(text, t.sectionStart)
	}
}

func (t *tokenizer) cleanup() {
	if t.sectionStart != t.index {
		switch {
		case t.state == sText || t.state == sInPlainText || (t.state == sInSpecialTag && t.sequenceIndex == 0):
			t.cbs.ontext(t.sectionStart, t.index)
			t.sectionStart = t.index
		case t.state == sInAttributeValueDq || t.state == sInAttributeValueSq || t.state == sInAttributeValueNq:
			t.cbs.onattribdata(t.sectionStart, t.index)
			t.sectionStart = t.index
		}
	}
}

func (t *tokenizer) run() {
	for t.index < len(t.buf) {
		c := t.buf[t.index]
		switch t.state {
		case sText:
			t.stateText(c)
		case sInPlainText:
			t.index = len(t.buf) - 1
		case sSpecialStartSequence:
			t.stateSpecialStartSequence(c)
		case sInSpecialTag:
			t.stateInSpecialTag(c)
		case sCDATASequence:
			t.stateCDATASequence(c)
		case sDeclarationSequence:
			t.stateDeclarationSequence(c)
		case sInAttributeValueDq:
			t.handleInAttributeValue(c, '"')
		case sInAttributeName:
			t.stateInAttributeName(c)
		case sInCommentLike:
			t.stateInCommentLike(c)
		case sInSpecialComment:
			t.stateInSpecialComment(c)
		case sBeforeAttributeName:
			t.stateBeforeAttributeName(c)
		case sInTagName:
			t.stateInTagName(c)
		case sInClosingTagName:
			t.stateInClosingTagName(c)
		case sBeforeTagName:
			t.stateBeforeTagName(c)
		case sAfterAttributeName:
			t.stateAfterAttributeName(c)
		case sInAttributeValueSq:
			t.handleInAttributeValue(c, '\'')
		case sBeforeAttributeValue:
			t.stateBeforeAttributeValue(c)
		case sBeforeClosingTagName:
			t.stateBeforeClosingTagName(c)
		case sAfterClosingTagName:
			t.stateAfterClosingTagName(c)
		case sInAttributeValueNq:
			t.stateInAttributeValueNoQuotes(c)
		case sInSelfClosingTag:
			t.stateInSelfClosingTag(c)
		case sInDeclaration:
			t.stateInDeclaration(c)
		case sBeforeDeclaration:
			t.stateBeforeDeclaration(c)
		case sBeforeComment:
			t.stateBeforeComment(c)
		case sInProcessingInstruction:
			// XML mode only.
		case sInEntity:
			t.stateInEntity()
		}
		t.index++
	}
	t.cleanup()
	t.finish()
}

func (t *tokenizer) finish() {
	if t.state == sInEntity {
		// The whole buffer was available, so stateInEntity already ran; keep
		// the base state for trailing-data handling.
		t.state = t.baseState
	}
	t.handleTrailingData()
	t.cbs.onend()
}

func (t *tokenizer) handleTrailingData() {
	end := len(t.buf)
	if t.state == sInCommentLike {
		if t.currentSeq == seqCdataEnd {
			t.cbs.oncomment(t.sectionStart-len(seqCdata)-1, end, 0)
		} else {
			offset := t.sequenceIndex
			if offset > len(seqCommentEnd)-1 {
				offset = len(seqCommentEnd) - 1
			}
			t.cbs.oncomment(t.sectionStart, end, offset)
		}
		return
	}
	switch t.state {
	case sBeforeDeclaration, sInSpecialComment, sBeforeComment, sCDATASequence:
		t.cbs.oncomment(t.sectionStart, end, 0)
		return
	case sDeclarationSequence:
		if t.sequenceIndex != len(seqDoctype) {
			t.cbs.oncomment(t.sectionStart, end, 0)
		}
		return
	case sInDeclaration:
		return
	}
	if t.sectionStart >= end {
		return
	}
	switch t.state {
	case sInTagName, sBeforeAttributeName, sBeforeAttributeValue, sAfterAttributeName, sInAttributeName,
		sInAttributeValueSq, sInAttributeValueDq, sInAttributeValueNq, sInClosingTagName:
		// An unfinished tag is dropped.
	default:
		t.cbs.ontext(t.sectionStart, end)
	}
}
