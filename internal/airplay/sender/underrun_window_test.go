package sender

import (
	"testing"

	"saundo/internal/audio/ring"
)

// TestUnderrunWindowUsesOutputFrames pins the unit of the source-recovery
// window: framesSent counts 44.1 kHz output frames, so a 48 kHz capture must
// not stretch the window past 100 ms and classify a real gap as a pause.
func TestUnderrunWindowUsesOutputFrames(t *testing.T) {
	s := New(&audioRecordIO{}, Events{}, nil)
	s.state = StateStreaming
	s.inputRate = 48000
	s.ring = ring.New(8192)
	s.ringFed = true
	s.lastFedFrames = 0
	s.framesSent = 4600 // 104 ms at the output rate, inside the old 4800 frame window
	s.pendingUnderruns = 1

	s.sendAudioPacket() // empty ring: a short fill

	s.ring.Push(make([]int16, 1000*2))
	s.framesSent = 4700 // still inside the old 4800 frame window
	s.sendAudioPacket() // a full packet closes the window

	if got := s.Stats().Underruns; got != 0 {
		t.Fatalf("underruns = %d, want 0: a 4700-frame gap is over the 100 ms output window", got)
	}
}
