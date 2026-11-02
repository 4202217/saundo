package locales

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestCatalogKeyParity keeps the translations complete. mygo i18n check
// warns about missing messages; this test fails.
func TestCatalogKeyParity(t *testing.T) {
	en := readCatalog(t, "en.json")
	zh := readCatalog(t, "zh-CN.json")
	for key := range en {
		if _, ok := zh[key]; !ok {
			t.Errorf("zh-CN.json is missing %q", key)
		}
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("en.json has no %q", key)
		}
	}
}

func readCatalog(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		if strings.HasPrefix(key, "@") {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			t.Fatalf("%s: %q is not a string: %v", name, key, err)
		}
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s: %q is empty", name, key)
		}
		out[key] = text
	}
	return out
}

// TestVolumeValueRoundsToWholePercent keeps the receiver's fractional
// feedback out of the readout.
func TestVolumeValueRoundsToWholePercent(t *testing.T) {
	for _, locale := range []string{"en", "zh-CN"} {
		for _, c := range []struct {
			v    float64
			want string
		}{{22.034, "22%"}, {65, "65%"}, {99.6, "100%"}} {
			got := Catalog.Format(locale, "flyoutVolumeValue", map[string]any{"v": c.v})
			if got != c.want {
				t.Errorf("%s: volume %v = %q, want %q", locale, c.v, got, c.want)
			}
		}
	}
}
