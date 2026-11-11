package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// TestCredentialResetNoticeAndDialog pins the flyout recovery surface: the
// reset bar renders, and clicking it opens the dialog with the explanation
// and both actions.
func TestCredentialResetNoticeAndDialog(t *testing.T) {
	a := previewApp()
	a.snap.CredentialReset = true
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	if !tt.HasText(locales.CredentialResetBar()) {
		t.Fatalf("the flyout is missing the recovery bar; texts: %q", tt.Texts())
	}
	if err := tt.Click(locales.CredentialResetBar()); err != nil {
		t.Fatalf("click the recovery bar: %v", err)
	}
	for _, want := range []string{
		locales.CredentialResetTitle(),
		locales.CredentialResetSub(),
		locales.CredentialResetBody(),
		locales.CredentialResetHint(),
		locales.CredentialResetDismiss(),
		locales.CredentialResetPair(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("the recovery dialog is missing %q; texts: %q", want, tt.Texts())
		}
	}
}
