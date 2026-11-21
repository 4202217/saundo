package engine

import (
	"errors"
	"testing"

	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
)

func TestValidateTargetRefusesAStaleCombo(t *testing.T) {
	e := &Engine{}
	tgt := receivers.Target{
		Device: device.Device{ID: "combo:01", Name: "书桌", StereoID: "combo:01", Online: true,
			Members: []device.Device{{}, {}}},
		Entry:   receivers.Entry{Key: "combo:01", Kind: receivers.KindCombo, Name: "书桌"},
		Invalid: true,
	}
	err := e.validateTarget(tgt)
	var pe *planError
	if !errors.As(err, &pe) || pe.key != ErrComboStale {
		t.Fatalf("validateTarget = %v, want ErrComboStale", err)
	}
}
