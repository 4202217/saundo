package app

import "saundo/internal/nowplaying"

func (a *App) hasNowSession(aumid string) bool {
	if aumid == "" {
		return false
	}
	for _, s := range a.nowSnap.Sessions {
		if s.AUMID == aumid {
			return true
		}
	}
	return false
}

func (a *App) nowMediaIndex() int {
	if len(a.nowSnap.Sessions) == 0 {
		return 0
	}
	for i, s := range a.nowSnap.Sessions {
		if s.AUMID != "" && s.AUMID == a.nowMediaAUMID {
			return i
		}
	}
	return snapIndex(a.nowSnap)
}

func (a *App) nowSession() (nowplaying.Session, bool) {
	if len(a.nowSnap.Sessions) > 0 {
		return a.nowSnap.Sessions[a.nowMediaIndex()], true
	}
	if a.nowOK && a.nowTrack.Title != "" {
		return nowplaying.Session{Title: a.nowTrack.Title, Artist: a.nowTrack.Artist, Album: a.nowTrack.Album}, true
	}
	return nowplaying.Session{}, false
}
