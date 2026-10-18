package vine

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

var schemas = map[string][]Field{
	"google":        {{"code", String{MinLength: 1, MaxLength: 4096}}, {"codeVerifier", String{MinLength: 43, MaxLength: 128}}, {"nonce", String{MinLength: 32, MaxLength: 128}}},
	"register":      {{"fullname", String{}}, {"email", String{Trim: true, Lower: true, Email: true}}, {"password", String{}}},
	"login":         {{"email", String{Trim: true, Lower: true, Email: true}}, {"password", String{}}},
	"checkEmail":    {{"email", String{Trim: true, Lower: true, Email: true}}},
	"resetPassword": {{"password", String{}}},
}

func emailCorpus() []string {
	corpus := []string{"", "@", "a@", "@b.co", "a@b", "a@b.c", "a@b.co", "a.b@c.co", "a..b@c.co", ".a@c.co", "a.@c.co", "a@b..co",
		"a@-b.co", "a@b-.co", "a@b_c.co", "a@b.123", "a@b.c0m", "a@b.xn--p1ai", "a@b.xn-", `"a b"@c.co`, `"a\"b"@c.co`, `"a\b"@c.co`, `"@c.co`,
		`""@c.co`, "a@b@c.co", "user+tag@gmail.com", "üñí@bücher.de", "a@bücher.co", "a@b.ü", "a@b.üü", "😀@b.co", "a@😀.co", "a@b.co ", " a@b.co",
		"a@ｂ.co", "a@b.ｃｏ", "a@b.co.", "a@[127.0.0.1]", "a@127.0.0.1", strings.Repeat("a", 64) + "@b.co", strings.Repeat("a", 65) + "@b.co",
		"a@" + strings.Repeat("b", 63) + ".co", "a@" + strings.Repeat("b", 64) + ".co", strings.Repeat("é", 33) + "@b.co", "A@B.CO",
		"a!#$%&'*+-/=?^_`{|}~@b.co", "a(b)@c.co", "a,b@c.co", "a\x00b@c.co", "a@b.c\to", "a@b.c o", "a@b.co\u00a0", "a@b.co\u2028"}
	alphabet := []rune("ab.@-_\"\\ +éü😀AZ09\t,ｂ")
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 3000; i++ {
		n := 1 + r.Intn(12)
		b := make([]rune, n)
		for j := range b {
			b[j] = alphabet[r.Intn(len(alphabet))]
		}
		corpus = append(corpus, string(b), string(b)+"@ex.co", "x@"+string(b)+".co", string(b)+".co")
	}
	return corpus
}

func TestVineInteroperability(t *testing.T) {
	type testCase struct {
		Schema string          `json:"schema"`
		Data   json.RawMessage `json:"data"`
	}
	cases := []testCase{}
	for schema := range schemas {
		for _, data := range []string{`{}`, `{"email":null,"password":1,"fullname":true}`, `{"email":" A@B.CO ","password":"p","fullname":"F"}`,
			`{"email":"bad","password":[],"fullname":{}}`, `{"email":"\u00a0x@y.id\ufeff","password":"","fullname":"x"}`,
			`{"code":"","codeVerifier":"` + strings.Repeat("v", 42) + `","nonce":"` + strings.Repeat("n", 129) + `"}`,
			`{"code":"c","codeVerifier":"` + strings.Repeat("😀", 22) + `","nonce":"` + strings.Repeat("n", 32) + `"}`,
			`{"code":"` + strings.Repeat("c", 4097) + `","codeVerifier":"` + strings.Repeat("v", 128) + `","nonce":"` + strings.Repeat("n", 128) + `"}`} {
			cases = append(cases, testCase{schema, json.RawMessage(data)})
		}
	}
	emails := emailCorpus()
	request, _ := json.Marshal(map[string]any{"cases": cases, "emails": emails})
	command := exec.Command("node", "../../scripts/vine-fixture.mjs")
	command.Stdin = bytes.NewReader(request)
	raw, err := command.Output()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var expected struct {
		Cases []struct {
			Value   map[string]string `json:"value"`
			Message string            `json:"message"`
			Issues  []Issue           `json:"issues"`
		} `json:"cases"`
		Emails []bool `json:"emails"`
	}
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		var input Input
		_ = json.Unmarshal(c.Data, &input)
		value, err := Validate(input, schemas[c.Schema]...)
		want := expected.Cases[i]
		if want.Issues != nil {
			got, ok := err.(*Error)
			if !ok || !reflect.DeepEqual(got.Issues, want.Issues) || got.Error() != want.Message {
				t.Errorf("%s %s: got %v %+v, want %+v", c.Schema, c.Data, value, err, want.Issues)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(value, want.Value) {
			t.Errorf("%s %s: got %v %v, want %v", c.Schema, c.Data, value, err, want.Value)
		}
	}
	t.Logf("validator.js accepted %d of %d emails", func() (n int) {
		for _, ok := range expected.Emails {
			if ok {
				n++
			}
		}
		return
	}(), len(emails))
	mismatches := 0
	for i, email := range emails {
		if IsEmail(email) != expected.Emails[i] {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("isEmail(%q) = %v, validator.js says %v", email, !expected.Emails[i], expected.Emails[i])
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("%d/%d email mismatches", mismatches, len(emails))
	}
}
