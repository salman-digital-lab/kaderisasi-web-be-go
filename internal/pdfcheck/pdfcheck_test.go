package pdfcheck

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"testing"
)

func TestMatchesPdfLib(t *testing.T) {
	out, err := exec.Command("node", "../../scripts/pdf-fixture.mjs", "generate").Output()
	if err != nil {
		t.Fatal(err)
	}
	var corpus []string
	_ = json.Unmarshal(out, &corpus)
	command := exec.Command("node", "../../scripts/pdf-fixture.mjs", "check")
	command.Stdin = bytes.NewReader(out)
	verdicts, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var want []bool
	_ = json.Unmarshal(verdicts, &want)
	for i, b64 := range corpus {
		data, _ := base64.StdEncoding.DecodeString(b64)
		if got := HasPages(data); got != want[i] {
			t.Errorf("pdf #%d (%d bytes): got %v, pdf-lib says %v", i, len(data), got, want[i])
		}
	}
}
