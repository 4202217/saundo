package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/engine"
)

// The 60-second strip under the measured card. It plots only what moves: the
// ring headroom and the two event kinds. The annotated line adds the three
// facts a bare sparkline lacks: a value at the right edge, a marked silent
// stretch so a zero is not read as starvation, and one line naming the latest
// event with its age and count.

const (
	connectionChartHeight = 46
	connectionChartGutter = 34
)

// previewStream supplies the strip in preview renders and tests, which run
// without an engine. Production always reads the engine history.
var previewStream func(dst []engine.StreamBucket) []engine.StreamBucket

func (a *App) streamBuckets(dst []engine.StreamBucket) []engine.StreamBucket {
	switch {
	case a.eng != nil:
		return a.eng.StreamBuckets(dst)
	case previewStream != nil:
		return previewStream(dst)
	}
	return nil
}

func (a *App) connectionStrip(c *ui.Context, p palette) {
	var buf [engine.StreamBucketCount]engine.StreamBucket
	buckets := a.streamBuckets(buf[:])
	summary := connectionEventSummary(buckets, time.Now())
	idle := streamHasIdle(buckets)
	ui.Column(c).FillWidth().Shrink(0).Children(func() {
		rowLine(c, p)
		ui.Column(c).Padding(8, 14, 10, 14).Gap(4).Children(func() {
			ui.Row(c).Height(14).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.SettingsConnectionStripWindow()).FontSize(10.5).TextColor(p.muted).SingleLine()
				ui.Spacer(c)
				connectionChartLegend(c, p, idle)
			})
			if summary != "" {
				ui.Text(c, summary).FontSize(10.5).TextColor(p.muted).
					FontFeatures("tnum").SingleLine().Ellipsis("…")
			}
			a.connectionChart(c, p)
		})
	})
}

// connectionEventSummary names the most recent second with an event. The
// counts are that second's; the card carries the window totals.
func connectionEventSummary(buckets []engine.StreamBucket, now time.Time) string {
	for i := len(buckets) - 1; i >= 0; i-- {
		b := buckets[i]
		if b.Underruns == 0 && b.Retransmits == 0 {
			continue
		}
		age := int(now.Sub(b.At).Seconds() + 0.5)
		if age < 1 {
			age = 1
		}
		if b.Underruns > 0 {
			return locales.SettingsConnectionRecentUnder(age, int(b.Underruns))
		}
		return locales.SettingsConnectionRecentRetr(age, int(b.Retransmits))
	}
	return ""
}

func streamHasIdle(buckets []engine.StreamBucket) bool {
	for _, b := range buckets {
		if b.Idle {
			return true
		}
	}
	return false
}

func connectionChartLegend(c *ui.Context, p palette, idle bool) {
	ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
		connectionLegendItem(c, p, p.fg2, 6, 2, locales.SettingsConnectionChartPending())
		if idle {
			connectionLegendItem(c, p, p.line, 6, 2, locales.SettingsConnectionHeadroomIdle())
		}
		connectionLegendItem(c, p, p.warning, 4, 4, locales.SettingsConnectionChartRetransmit())
		connectionLegendItem(c, p, p.red, 2, 4, locales.SettingsConnectionUnderrun())
	})
}

func connectionLegendItem(c *ui.Context, p palette, color ui.Color, w, h int, label string) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(float32(w), float32(h)).Radius(float32(w) / 2).Background(color)
		ui.Text(c, label).FontSize(10.5).TextColor(p.muted).SingleLine()
	})
}

func (a *App) connectionChart(c *ui.Context, p palette) {
	ui.Box(c).FillWidth().Height(connectionChartHeight).Shrink(0).Draw(func(pt *ui.Painter, r ui.Rect) {
		var buf [engine.StreamBucketCount]engine.StreamBucket
		drawConnectionChart(pt, p, r, a.streamBuckets(buf[:]))
		if phaseOf(a.snap) == phaseStreaming {
			// The history samples at 10 Hz, so more frames would repeat data.
			pt.AnimationFrameRate(10)
		}
	})
}

func drawConnectionChart(pt *ui.Painter, p palette, r ui.Rect, buckets []engine.StreamBucket) {
	plotX := r.X + connectionChartGutter
	plotW := r.W - connectionChartGutter
	if plotW <= 0 {
		return
	}
	top, bottom := r.Y+6, r.Y+38
	n := len(buckets)
	bw := float32(0)
	if n > 0 {
		bw = plotW / float32(n)
	}
	// The silent stretches: a faint band and one label mark a dry ring that
	// is idle, not starved.
	for i := 0; i < n; {
		if !buckets[i].Idle {
			i++
			continue
		}
		j := i
		for j < n && buckets[j].Idle {
			j++
		}
		x0 := plotX + float32(i)*bw
		w := float32(j-i) * bw
		pt.Fill(ui.Rect{X: x0, Y: top, W: w, H: bottom - top}, p.line.Alpha(0.32), 0)
		if w > 26 {
			pt.Text(x0+3, top+2, locales.SettingsConnectionHeadroomIdle(), 9.5, p.faint)
		}
		i = j
	}
	pt.Text(r.X, r.Y, "50 ms", 10, p.faint)
	pt.Text(r.X, bottom-10, "0", 10, p.faint)
	for x := plotX; x < plotX+plotW; x += 6 {
		pt.Fill(ui.Rect{X: x, Y: top, W: 3, H: 1}, p.muted.Alpha(0.45), 0)
	}
	pt.Fill(ui.Rect{X: plotX, Y: bottom, W: plotW, H: 1}, p.line, 0)
	if n == 0 {
		return
	}
	prevY, havePrev := float32(0), false
	for i, b := range buckets {
		x0 := plotX + float32(i)*bw
		x1 := x0 + bw
		if b.Valid {
			head := float32(b.HeadMS)
			if head < 0 {
				head = 0
			}
			if head > 50 {
				head = 50
			}
			y := bottom - head/50*(bottom-top)
			if havePrev {
				pt.Line(x0, prevY, x0, y, 1, p.fg2.Alpha(0.8))
			}
			pt.Line(x0, y, x1, y, 1, p.fg2.Alpha(0.8))
			prevY, havePrev = y, true
		} else {
			havePrev = false
		}
		if b.Retransmits > 0 {
			h := float32(b.Retransmits) / 8 * 10
			if h > 10 {
				h = 10
			} else if h < 2 {
				h = 2
			}
			pt.Fill(ui.Rect{X: x0 + bw/2 - 2, Y: bottom - h, W: 4, H: h}, p.warning, 0.5)
		}
		count := b.Underruns
		if count > 3 {
			count = 3
		}
		for k := uint64(0); k < count; k++ {
			pt.Fill(ui.Rect{X: x0 + bw/2 + 1 + float32(k)*2.2, Y: bottom + 1, W: 2, H: 5}, p.red, 0)
		}
	}
	// The right edge carries the current value; an idle stretch says nothing.
	for i := n - 1; i >= 0; i-- {
		b := buckets[i]
		if !b.Valid || b.Idle {
			continue
		}
		head := b.HeadMS
		if head < 0 {
			head = 0
		}
		if head > 50 {
			head = 50
		}
		y := bottom - float32(head)/50*(bottom-top) - 4
		if y < r.Y+9 {
			y = r.Y + 9
		}
		pt.Text(r.X+r.W-30, y, fmt.Sprintf("%d ms", int(head+0.5)), 9, p.fg2)
		break
	}
}
