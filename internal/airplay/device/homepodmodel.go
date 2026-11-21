package device

// homePodModels maps the HomePod model identifiers Apple advertises to their
// marketing names. Source: EveryMac's Apple HomePod Models table, reviewed
// 2026-11-21. There are four models; an identifier outside this table falls
// back to the HomePod family in the UI rather than guessing a generation.
var homePodModels = map[string]string{
	"AudioAccessory1,1":  "HomePod",
	"AudioAccessory5,1":  "HomePod mini",
	"AudioAccessory6,1":  "HomePod (2nd gen)",
	"AudioAccessory11,1": "HomePod mini 2",
}

// HomePodModelName returns the marketing name for a known HomePod identifier,
// or "" when the identifier is not one of the four.
func HomePodModelName(model string) string {
	return homePodModels[model]
}
