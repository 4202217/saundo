package device

import "testing"

// TestMacFormFor pins the model table against the identifiers real Macs
// advertise. Mac14,2 is the case that exposed the missing table: it is a
// MacBook Air, not a desktop.
func TestMacFormFor(t *testing.T) {
	tests := []struct {
		model string
		want  MacForm
	}{
		{"Mac14,2", MacFormLaptop},
		{"MacBookPro18,3", MacFormLaptop},
		{"MacBookAir10,1", MacFormLaptop},
		{"Mac17,2", MacFormLaptop},
		{"Mac17,6", MacFormLaptop},
		{"Macmini9,1", MacFormMini},
		{"Mac14,3", MacFormMini},
		{"Mac16,10", MacFormMini},
		{"Mac13,1", MacFormStudio},
		{"Mac14,13", MacFormStudio},
		{"Mac15,14", MacFormStudio},
		{"Mac17,14", MacFormStudio},
		{"iMac21,1", MacFormDesktop},
		{"MacPro7,1", MacFormDesktop},
		{"Mac14,8", MacFormDesktop},
		{"Mac99,99", MacFormDesktop},
		{"MacBookPro99,9", MacFormLaptop},
		{"Macmini99,9", MacFormMini},
		{"MacStudio99,9", MacFormStudio},
		{"AppleTV6,2", MacFormUnknown},
		{"", MacFormUnknown},
	}
	for _, tc := range tests {
		if got := MacFormFor(tc.model); got != tc.want {
			t.Errorf("MacFormFor(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestMacModelName(t *testing.T) {
	if got, want := MacModelName("Mac14,2"), "MacBook Air (M2, 2022)"; got != want {
		t.Errorf("MacModelName(Mac14,2) = %q, want %q", got, want)
	}
	if got := MacModelName("Mac99,99"); got != "" {
		t.Errorf("MacModelName(Mac99,99) = %q, want empty", got)
	}
}
