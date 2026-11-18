package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/engine"
)

// noticeLifetime is how long a session notice stays before it fades on its
// own. The user can close it sooner; a new event replaces it.
const noticeLifetime = 8 * time.Second

// sessionNoticeState is the flyout's bookkeeping for transient notices: the
// engine notice the user dismissed, its auto-dismiss timer, and the joining
// target whose derived notice the user closed.
type sessionNoticeState struct {
	dismissed  string
	timer      *time.Timer
	closedJoin string
}

// sessionNotice is one notice ready to render.
type sessionNotice struct {
	text      string
	retry     func()
	join      bool
	joinID    string
	signature string
}

// noticeSignature identifies one engine notice so a dismissal survives the
// publishes that follow it.
func noticeSignature(n engine.SessionNotice) string {
	return fmt.Sprintf("%d:%s:%d", n.Kind, n.DeviceID, n.Remaining)
}

// joiningMember finds the target the current connect is opening.
func (a *App) joiningMember() (sessionMember, bool) {
	for _, m := range a.sessionMembers() {
		if m.HasTarget && m.Target.State == engine.TargetJoining {
			return m, true
		}
	}
	return sessionMember{}, false
}

// sessionNotice picks what the flyout explains now: a join in progress takes
// precedence, then an engine member-lost notice the user has not dismissed.
func (a *App) sessionNotice() (sessionNotice, bool) {
	if phaseOf(a.snap) == phaseConnecting {
		if joining, ok := a.joiningMember(); ok && len(a.sessionMembers()) > 1 &&
			a.notice.closedJoin != joining.Device.ID {
			return sessionNotice{
				text:   locales.FlyoutJoinNotice(joining.Device.Name),
				join:   true,
				joinID: joining.Device.ID,
			}, true
		}
	}
	n := a.snap.Notice
	if n.Kind == engine.NoticeNone {
		return sessionNotice{}, false
	}
	sig := noticeSignature(n)
	if a.notice.dismissed == sig {
		return sessionNotice{}, false
	}
	id := n.DeviceID
	return sessionNotice{
		text:      locales.FlyoutMemberLostNotice(n.Name),
		signature: sig,
		retry: func() {
			a.connectDevice(id)
		},
	}, true
}

// noticeShown reports whether the flyout carries a session notice now.
func (a *App) noticeShown() bool {
	_, ok := a.sessionNotice()
	return ok
}

// trackNotice runs on every snapshot: it resets the per-session bookkeeping and
// starts the auto-dismiss timer for a new engine notice.
func (a *App) trackNotice() {
	if _, joining := a.joiningMember(); !joining || phaseOf(a.snap) != phaseConnecting {
		a.notice.closedJoin = ""
	}
	n := a.snap.Notice
	if n.Kind == engine.NoticeNone {
		a.notice.dismissed = ""
		if a.notice.timer != nil {
			a.notice.timer.Stop()
			a.notice.timer = nil
		}
		return
	}
	sig := noticeSignature(n)
	if a.notice.dismissed == sig || a.notice.timer != nil {
		return
	}
	a.notice.timer = time.AfterFunc(noticeLifetime, func() {
		a.updateUI(func() {
			if noticeSignature(a.snap.Notice) == sig {
				a.notice.dismissed = sig
			}
			a.notice.timer = nil
		})
	})
}

// dismissNotice closes the current notice until the next event.
func (a *App) dismissNotice() {
	n, ok := a.sessionNotice()
	if !ok {
		return
	}
	if n.join {
		a.notice.closedJoin = n.joinID
	} else {
		a.notice.dismissed = n.signature
	}
	if a.notice.timer != nil {
		a.notice.timer.Stop()
		a.notice.timer = nil
	}
}

// sessionNoticeBar is the line under the device card. A lost member explains
// why the room paused and offers a retry; a join in progress warns that the
// other devices pause briefly. Both share one shape.
func (a *App) sessionNoticeBar(c *ui.Context, p palette) {
	n, ok := a.sessionNotice()
	if !ok {
		return
	}
	tone := barAccent
	if n.join {
		tone = barNeutral
	}
	statusBar(c, p, tone, iconInfo, n.text, func() {
		if n.retry != nil {
			b := ui.ButtonBase(c.Key("notice-retry")).Height(26).Radius(7).Padding(0, 8).
				Center().Shrink(0).Label(locales.ActionRetry()).Cursor(ui.CursorPointer)
			b.OnClick(n.retry)
			hoverFill(b, p.pill, 7, 0)
			b.Children(func() {
				ui.Text(c, locales.ActionRetry()).FontSize(11.5).FontWeight(500).TextColor(p.accent).SingleLine()
			})
		}
		close := ui.ButtonBase(c.Key("notice-dismiss")).Size(26, 26).Radius(7).Center().
			Shrink(0).Label(locales.FlyoutNoticeDismiss()).Cursor(ui.CursorPointer)
		close.OnClick(func() { a.dismissNotice() })
		hoverFill(close, p.pill, 7, 0)
		close.Children(func() {
			ui.Icon(c, iconClose).Size(12, 12).TextColor(p.muted)
		})
	})
}
