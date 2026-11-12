//go:build darwin

package darwin

import (
	"bytes"
	"image/png"
	"testing"
)

// TestAppIconLive renders Finder's icon through both entry points the capture
// picker and the Now Playing card use.
func TestAppIconLive(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"bundle", AppIconForBundleID("com.apple.finder")},
		{"path", AppIconForPath("/System/Library/CoreServices/Finder.app")},
	} {
		if len(tc.data) == 0 {
			t.Fatalf("%s: no icon", tc.name)
		}
		if _, err := png.Decode(bytes.NewReader(tc.data)); err != nil {
			t.Fatalf("%s: not a PNG: %v", tc.name, err)
		}
	}
}
