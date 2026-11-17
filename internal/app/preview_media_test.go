//go:build preview

package app

import "testing"

func TestPreviewHasMedia(t *testing.T) {
	a := PreviewApp()
	s, ok := a.nowSession()
	if !ok || s.Title == "" {
		t.Fatal("the preview has no media session")
	}
	if a.nowArt == nil || !a.nowLampOK || a.nowFieldLight == nil || a.levelFrame == nil {
		t.Fatal("the media fixture is incomplete")
	}
}
