package device

// appleTVModels maps the Apple TV model identifiers to their marketing names.
// Source: EveryMac's All Apple TV Models table, reviewed 2026-11-21. The
// identifiers distinguish generations; an identifier outside the table falls
// back to the Apple TV family in the UI.
var appleTVModels = map[string]string{
	"AppleTV1,1":  "Apple TV (1st gen)",
	"AppleTV2,1":  "Apple TV (2nd gen)",
	"AppleTV3,1":  "Apple TV (3rd gen)",
	"AppleTV3,2":  "Apple TV (3rd gen)",
	"AppleTV5,3":  "Apple TV HD",
	"AppleTV6,2":  "Apple TV 4K (1st gen)",
	"AppleTV11,1": "Apple TV 4K (2nd gen)",
	"AppleTV14,1": "Apple TV 4K (3rd gen)",
	"AppleTV18,1": "Apple TV 4K (4th gen)",
}

// AppleTVModelName returns the marketing name for a known Apple TV identifier,
// or "" when the identifier is not in the table.
func AppleTVModelName(model string) string {
	return appleTVModels[model]
}
