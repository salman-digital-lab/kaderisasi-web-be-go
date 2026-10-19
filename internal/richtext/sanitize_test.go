package richtext

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

var handwritten = []string{
	"", "plain", "a & b < c > d \" e ' f", "&amp; &lt; &gt; &quot; &#39; &#x27; &nbsp; &copy &copyx &notin; &noti &amp", "&#0; &#128; &#x110000; &#55296; &#x41 &#65;x &# &#x &bogus;",
	`<p style="text-align: center">Center</p>`, `<p style="text-align:right;">r</p>`, `<h2 style="TEXT-ALIGN: left; color: red">h</h2>`,
	`<p style="text-align: justify !important">j</p>`, `<p style="text-align: center; text-align: left">x</p>`, `<p style="">e</p>`,
	`<p style="text-align: center;color: red: blue">x</p>`, `<p style="color: 'a;b'; text-align: left">x</p>`, `<p style="text-align: /*c*/ center">x</p>`,
	`<p style="text-align: cen/*c*/ter">x</p>`, `<p style="a{b:c}; text-align: left">x</p>`, `<p style="text-align: center}">x</p>`, `<p style="foo">x</p>`,
	`<p style="*text-align: left">x</p>`, `<p style="@media x; text-align: right">x</p>`, `<p style="text-align: (center">x</p>`,
	`<a href="https://example.com/a?b=1&amp;c=2" title="T">link</a>`, `<a href="javascript:alert(1)">x</a>`, `<a href="JaVaScRiPt:alert(1)">x</a>`,
	"<a href=\"java\tscript:alert(1)\">x</a>", `<a href="java<!-- -->script:alert(1)">x</a>`, `<a href="//evil.example">x</a>`, `<a href="\\evil">x</a>`,
	`<a href="mailto:a@b.c">m</a>`, `<a href="tel:123">t</a>`, `<a href="/relative">r</a>`, `<a href="">empty</a>`, `<a href title>bool</a>`,
	`<a href="x" href="y">dup</a>`, `<a HREF="https://UPPER">u</a>`, `<a href='single'>s</a>`, `<a href=unquoted>u</a>`, `<a 1="a" href="h">n</a>`,
	`<a href="&#106;avascript:alert(1)">e</a>`, `<a href="&copy=1">attr entity</a>`, `<a href="?a=1&copy;b">semi</a>`,
	`<script>alert(1)</script>after`, `<style>p{}</style>x`, `<textarea><b>t</b></textarea>`, `<option>o<b>b</b></option>z`, `<xmp><b></xmp>`,
	`<title>a &amp; b</title>`, `<iframe><b>&amp;</b></iframe>`, `<noembed>n</noembed>`, `<plaintext><b>rest`,
	`<div><p>a<p>b</div>c`, `<ul><li>1<li>2</ul>`, `<p>unclosed`, `</p>`, `</br>`, `a</b>c`, `<b><i>x</b></i>`, `<br/><hr/><br>`, `<p/>after`,
	`<img src="x" onerror="alert(1)">`, `<svg><p>in svg</p><foreignobject><p>f</p></foreignobject></svg>`, `<math><mi>x</mi></math>`, `<svg><![CDATA[cd]]></svg>`,
	`<!-- comment --> <!DOCTYPE html> <?pi?> <![CDATA[x]]> <!--unclosed`, `<! bogus>`, `< p>`, `<3 hearts`, `a<`, `<p`, `<p class="x`, `</ p>`,
	`<form><form><p>f</p></form></form>`, `<table><tr><td>c</td></tr></table>`, `<h1>t<h2>u</h1>`, `<pre><code>x &lt; y</code></pre>`,
	`<blockquote><p>q</p></blockquote>`, `<s>strike</s><u>u</u><em>e</em><strong>s</strong>`, `<image src=x>`,
	strings.Repeat("<b>", 25) + "deep" + strings.Repeat("</b>", 25), "<p>emoji 😀 and é</p>", "<p>\u00a0nbsp\u2028ls</p>",
}

func generated(n int) []string {
	parts := []string{"<p>", "</p>", "<b>", "</b>", "<a href=\"", "<a href='", "\">", "'>", "</a>", "<br>", "<hr/>", "<script>", "</script>", "<div>",
		"</div>", "<li>", "<ul>", "</ul>", "<!--", "-->", "&amp;", "&", "&copy", "&#", "&#x3c;", "<", ">", "\"", "'", "=", " ", "\n", "text",
		"javascript:", "https://e.x/", "style=\"text-align:center\"", "<p style=\"", "text-align: right", ";", "<h1>", "</h1>", "<svg>", "</svg>",
		"<title>", "</title>", "<textarea>", "</textarea>", "/", "é", "😀", "<x-y>", "<!DOCTYPE", "<![CDATA[", "]]>", "title=", "</ p>", "<p/>"}
	r := rand.New(rand.NewSource(fuzzSeed))
	out := make([]string, n)
	for i := range out {
		var b strings.Builder
		for j := 0; j < 1+r.Intn(14); j++ {
			b.WriteString(parts[r.Intn(len(parts))])
		}
		out[i] = b.String()
	}
	return out
}

func TestSanitizeMatchesSanitizeHTML(t *testing.T) {
	inputs := append(append([]string{}, handwritten...), generated(fuzzCount)...)
	raw, _ := json.Marshal(inputs)
	command := exec.Command("node", "../../scripts/sanitize-fixture.mjs")
	command.Stdin = bytes.NewReader(raw)
	out, err := command.Output()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var want []string
	if err := json.Unmarshal(out, &want); err != nil {
		t.Fatal(err)
	}
	t.Logf("compared %d inputs (seed %d)", len(inputs), fuzzSeed)
	mismatches := 0
	for i, in := range inputs {
		if got := Sanitize(in); got != want[i] {
			mismatches++
			if mismatches <= 25 {
				t.Errorf("Sanitize(%q)\n got  %q\n want %q", in, got, want[i])
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("%d/%d mismatches", mismatches, len(inputs))
	}
}

// RICHTEXT_FUZZ=<count> widens the generated corpus for one-off runs.
var fuzzSeed, fuzzCount = func() (int64, int) {
	if n, err := strconv.Atoi(os.Getenv("RICHTEXT_FUZZ")); err == nil && n > 0 {
		return time.Now().UnixNano(), n
	}
	return 11, 20000
}()
