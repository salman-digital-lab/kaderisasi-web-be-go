package mail

import (
	"strings"
	"testing"
)

func TestEmailChangeMessages(t *testing.T) {
	code := EmailChangeCodeHTML("042917", `x"<b>@example.test`)
	if !strings.Contains(code, "042917") || strings.Contains(code, "<b>") || !strings.Contains(code, "&lt;b&gt;") {
		t.Fatalf("code email does not show the code or escape the email:\n%s", code)
	}
	link := EmailChangeConfirmHTML("https://kaderisasi.salmanitb.com/confirm-email?token=a-b_c")
	if !strings.Contains(link, `href="https://kaderisasi.salmanitb.com/confirm-email?token=a-b_c"`) {
		t.Fatalf("confirm email link missing:\n%s", link)
	}
	if bad := EmailChangeConfirmHTML("javascript:alert(1)"); strings.Contains(bad, "javascript:") {
		t.Fatal("unsafe link kept")
	}
	if done := EmailChangedHTML("baru@example.test"); !strings.Contains(done, "baru@example.test") {
		t.Fatal("notice misses the new email")
	}
}
