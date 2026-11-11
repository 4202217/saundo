package engine

import (
	"os"
	"strings"
	"testing"
	"unicode"
)

// TestUserErrorsDoNotHardcodeCJK keeps the engine and the sender from
// rebuilding a Chinese sentence behind the catalog's back. The UI resolves
// the keys; a literal here would leak into an English install.
func TestUserErrorsDoNotHardcodeCJK(t *testing.T) {
	for _, file := range []string{"engine.go", "receivers_cmd.go", "../airplay/sender/sender.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, "lastError =") && !strings.Contains(line, `fail("`) {
				continue
			}
			for _, r := range line {
				if unicode.Is(unicode.Han, r) {
					t.Errorf("%s:%d still builds a user-facing error in code: %s", file, i+1, strings.TrimSpace(line))
					break
				}
			}
		}
	}
}
