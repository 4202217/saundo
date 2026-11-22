package engine

// trackInfo is the last Now Playing payload. A rebuilt cohort replays it, the
// way the lamp image is replayed, so an add does not leave every receiver
// without the current track until the next media change.
type trackInfo struct {
	set       bool
	title     string
	artist    string
	album     string
	cover     []byte
	coverMIME string
}

// remember stores the payload. The cover is copied because the caller owns
// its buffer and may reuse it.
func (t *trackInfo) remember(title, artist, album string, cover []byte, coverMIME string) {
	t.set = true
	t.title, t.artist, t.album = title, artist, album
	t.cover = append([]byte(nil), cover...)
	t.coverMIME = coverMIME
}

func (t *trackInfo) replay(tr transport) {
	if !t.set {
		return
	}
	tr.SetNowPlaying(t.title, t.artist, t.album, t.cover, t.coverMIME)
}
