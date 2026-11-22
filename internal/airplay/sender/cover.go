package sender

import (
	"bytes"
	"time"

	"saundo/internal/airplay/crypto"
)

// firstCoverRetry re-sends the first cover of a pair session after the
// follower's media-system sync has had time to establish the Now Playing item.
// The first send can land before that and be acknowledged without reaching the
// follower's LED renderer.
const firstCoverRetry = 2 * time.Second

// SetLampArtwork stores the ambient-light image and whether it owns the cover
// slot. While on, the light wins over the receiver track-info cover; while
// off, the media cover wins when present and this image (the origin reset) is
// the fallback. An unchanged choice is a no-op; a reconnect resends the
// effective cover.
func (s *Sender) SetLampArtwork(data []byte, mime string, on bool) {
	if bytes.Equal(data, s.lampCover) && mime == s.lampCoverMIME && on == s.lampOn {
		s.info("ambient light: unchanged (%d bytes, on=%v)", len(data), on)
		return
	}
	s.lampCover = append(s.lampCover[:0], data...)
	s.lampCoverMIME = mime
	s.lampOn = on
	s.info("ambient light: %d bytes, on=%v, state=%s", len(data), on, s.state)
	if s.state == StateStreaming {
		s.sendCover()
	}
}

// Playback states the receiver's MediaRemote accepts for the top display.
const (
	playbackPlaying = 1
	playbackPaused  = 2
)

// SetPlaybackState mirrors the media playback state to the receiver. Paused
// stops the Now Playing renderer and turns the top display off while the RTP
// stream keeps running; playing restores the current image. The state is
// stored and sent when the session streams.
func (s *Sender) SetPlaybackState(playing bool) {
	state := playbackPaused
	if playing {
		state = playbackPlaying
	}
	prev := s.playbackState
	s.playbackState = state
	if s.state != StateStreaming {
		return
	}
	s.sendPlaybackState()
	// A resume re-sends the artwork after the state: some receivers restart
	// the Now Playing renderer on state 1 and drop the image that arrived
	// before it.
	if state == playbackPlaying && prev == playbackPaused {
		s.sentCover = s.sentCover[:0]
		s.sentCoverMIME = ""
		s.sendCover()
	}
}

// sendPlaybackState posts the MR playback-state command on the encrypted
// control channel. The receiver applies it to the main session.
func (s *Sender) sendPlaybackState() {
	if s.state != StateStreaming || s.playbackState == 0 {
		return
	}
	if s.playbackState == s.playbackStateSent {
		// A new session assumes state 1, so the first light sends no state
		// command. The line makes that default visible when it matters.
		s.info("playback state: %d unchanged, not sent", s.playbackState)
		return
	}
	body, err := crypto.Encode(map[string]any{
		"type": "updateMRPlaybackState",
		"params": map[string]any{
			"mrPlaybackState": s.playbackState,
		},
	})
	if err != nil {
		s.warn("playback state encode: %v", err)
		return
	}
	s.sendRequest("POST", "/command", "application/x-apple-binary-plist", string(body), s.metadataHeaders())
	s.playbackStateSent = s.playbackState
	s.info("playback state: %d", s.playbackState)
}

func (s *Sender) coverImage() ([]byte, string) {
	if s.lampOn && len(s.lampCover) > 0 {
		return s.lampCover, s.lampCoverMIME
	}
	if len(s.mediaCover) > 0 {
		return s.mediaCover, s.mediaCoverMIME
	}
	return s.lampCover, s.lampCoverMIME
}

// sendCover pushes the effective cover as a Now Playing image. The receiver
// renders that cover on its top display; there is no separate artwork POST.
func (s *Sender) sendCover() {
	if s.state != StateStreaming {
		return
	}
	img, mime := s.coverImage()
	if len(img) == 0 || len(img) > 8*1024*1024 {
		return
	}
	if mime == "" {
		mime = "image/jpeg"
	}
	if bytes.Equal(img, s.sentCover) && mime == s.sentCoverMIME {
		return
	}
	// The top display needs a Now Playing item to attach the artwork to.
	// When the app has no track metadata, send a minimal one.
	if s.npTitle == "" && s.npArtist == "" && s.npAlbum == "" {
		s.sendRequest("SET_PARAMETER", s.rtspURI(), "application/x-dmap-tagged",
			dmapTag("mlit", dmapTag("minm", "saundo")), s.metadataHeaders())
	}
	s.sendRequest("SET_PARAMETER", s.rtspURI(), mime, string(img), s.metadataHeaders())
	s.sentCover = append(s.sentCover[:0], img...)
	s.sentCoverMIME = mime
	s.info("cover: %d bytes, lamp=%v", len(img), s.lampOn)
	if s.groupUUID != "" && !s.coverRetryDone {
		s.coverRetryDone = true
		s.coverRetry.arm(s.now())
	}
}

// onCoverRetry re-sends the effective cover once after a pair session's first
// light. The follower's top display takes the session's first artwork only
// after the pair's media-system sync has established the Now Playing item; the
// first send can land before that and be acknowledged without reaching the LED
// renderer. Re-sending the same image after the sync settles recovers it
// without a new session.
func (s *Sender) onCoverRetry() {
	if s.state != StateStreaming {
		return
	}
	s.sentCover = s.sentCover[:0]
	s.sentCoverMIME = ""
	s.sendCover()
	s.info("cover: first-light retry")
}
