package forms

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os/exec"
	"sort"
	"testing"
)

func TestLocaleCompareMatchesNode(t *testing.T) {
	alphabet := []string{"a", "B", "b", "A", "_", "-", "1", "9", "Z", "z", " ", "é", "E", ".", "Id", "id", "ID", "section_name", "sectionId", "maxSizeMB", "max_files", "x10", "x9"}
	r := rand.New(rand.NewSource(9))
	lists := [][]string{{"section_name", "sectionId", "id", "fields", "navigation", "accessMode", "settings", "version", "maxFiles", "maxSizeMB", "accept", "customMessage", "defaultTarget", "optionValue", "questionKey", "helpText", "defaultValue"}}
	for i := 0; i < 400; i++ {
		n := 2 + r.Intn(8)
		list := make([]string, n)
		for j := range list {
			k := ""
			for l := 0; l < 1+r.Intn(3); l++ {
				k += alphabet[r.Intn(len(alphabet))]
			}
			list[j] = k
		}
		lists = append(lists, list)
	}
	raw, _ := json.Marshal(lists)
	command := exec.Command("node", "../../scripts/collate-fixture.mjs")
	command.Stdin = bytes.NewReader(raw)
	out, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var want [][]string
	_ = json.Unmarshal(out, &want)
	bad := 0
	for i, list := range lists {
		got := append([]string(nil), list...)
		sort.SliceStable(got, func(a, b int) bool { return localeCompare.CompareString(got[a], got[b]) < 0 })
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want[i])
		if !bytes.Equal(g, w) {
			bad++
			if bad < 10 {
				t.Errorf("got %s want %s", g, w)
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d/%d lists differ", bad, len(lists))
	}
}
