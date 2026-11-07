package mail

import (
	_ "embed"
	"html/template"
	"strings"
)

//go:embed templates/account_notice.html
var accountNoticeSource string

var accountNotice = template.Must(template.New("account_notice").Parse(accountNoticeSource))

// Notice is a short account email: a heading, paragraphs, and optionally a
// code or a button. Every value is HTML-escaped by html/template.
type Notice struct {
	Title, Heading string
	Paragraphs     []string
	Code           string
	ButtonURL      string
	ButtonLabel    string
	Footnote       string
}

func NoticeHTML(n Notice) string {
	var b strings.Builder
	if err := accountNotice.Execute(&b, n); err != nil {
		// The template and its fields are fixed; this cannot fail at runtime.
		panic(err)
	}
	return b.String()
}

// EmailChangeCodeHTML is sent to the current email of a member without a
// password who asked to change it.
func EmailChangeCodeHTML(code, newEmail string) string {
	return NoticeHTML(Notice{
		Title:   "Kode perubahan email",
		Heading: "Kode perubahan email",
		Paragraphs: []string{
			"Ada permintaan untuk mengganti email akun Kaderisasi Masjid Salman ITB Anda menjadi " + newEmail + ".",
			"Masukkan kode berikut di halaman profil untuk melanjutkan. Kode berlaku 15 menit.",
		},
		Code:     code,
		Footnote: "Jika Anda tidak meminta perubahan ini, abaikan email ini. Email akun Anda tidak akan berubah.",
	})
}

// EmailChangeConfirmHTML is sent to the new email; the change applies only
// after the link is opened.
func EmailChangeConfirmHTML(confirmURL string) string {
	return NoticeHTML(Notice{
		Title:   "Konfirmasi email baru",
		Heading: "Konfirmasi email baru",
		Paragraphs: []string{
			"Klik tombol di bawah ini untuk menjadikan email ini sebagai email akun Kaderisasi Masjid Salman ITB Anda. Tautan berlaku 60 menit.",
		},
		ButtonURL:   confirmURL,
		ButtonLabel: "Konfirmasi email",
		Footnote:    "Jika Anda tidak meminta perubahan ini, abaikan email ini.",
	})
}

// EmailChangedHTML tells the previous email that the account moved.
func EmailChangedHTML(newEmail string) string {
	return NoticeHTML(Notice{
		Title:   "Email akun telah diubah",
		Heading: "Email akun telah diubah",
		Paragraphs: []string{
			"Email akun Kaderisasi Masjid Salman ITB Anda telah diubah menjadi " + newEmail + ". Mulai sekarang, masuk dengan email tersebut.",
		},
		Footnote: "Jika Anda tidak melakukan perubahan ini, segera hubungi pengurus Kaderisasi Masjid Salman ITB.",
	})
}
