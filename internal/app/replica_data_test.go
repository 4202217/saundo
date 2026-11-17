//go:build preview

package app

// The landing page's demo dataset, generated from the same fixture and catalog
// the preview matrix renders. Ported from the render-demo research branch at
// f3f6bf8 and rebuilt on Fixture and the catalog.

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/lamp"
	"saundo/internal/settings"
)

// replicaDataPath is the one demo dataset both sides read: the landing page
// loads it, and the fidelity checks below compare against the fixture that
// produces it. It sits with the site because the page has to fetch it as a
// plain script, which works from disk as well as over HTTP.
const replicaDataPath = "../../site/assets/demo-data.js"

// replicaData is what the landing page's demo shows. The receivers, the
// volume, the latency tier, the lamp, and the initial session state come from
// the fixture's streaming scenario; the sessions are a corpus — several
// players, each with its own list — because the card's arrow switches between
// players and the transport moves inside one. Values stay language-neutral so
// the page localises its own labels.
type replicaData struct {
	Receivers    []replicaReceiver    `json:"receivers"`
	Sessions     []replicaPlayer      `json:"sessions"`
	Volume       int                  `json:"volume"`
	Latency      string               `json:"latency"`
	LatencyModes []replicaLatencyMode `json:"latencyModes"`
	CustomMS     int                  `json:"customLatency"`
	State        replicaState         `json:"state"`
	Light        replicaLight         `json:"light"`
	Lamp         *replicaLamp         `json:"lamp,omitempty"`
	Templates    []replicaTemplate    `json:"templates"`
	Scenarios    []string             `json:"scenarios"`
}

// replicaState is the flyout's opening state, in the page's own vocabulary.
type replicaState struct {
	Phase    string `json:"phase"` // streaming, connecting, or idle
	Standby  bool   `json:"standby"`
	Applying bool   `json:"applying"`
	Pairing  bool   `json:"pairing"`
	PTP      bool   `json:"ptp"`
}

// replicaLight is the lamp's source and preset, as the page's `lamp` holds it.
type replicaLight struct {
	Off    bool   `json:"off"`
	Cover  bool   `json:"cover"`
	Preset string `json:"preset"`
}

type replicaLamp struct {
	Cover bool          `json:"cover"` // true: the light follows the cover, false: a template
	Base  string        `json:"base"`
	Blobs []replicaBlob `json:"blobs"`
}

type replicaBlob struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	R     float64 `json:"r"`
	Color string  `json:"color"`
	Alpha float64 `json:"alpha"`
}

// replicaTemplate is one ambient-light template: the preset's three colors and
// seed, plus the names both catalogs give it.
type replicaTemplate struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	NameEN string   `json:"nameEn"`
	Colors []string `json:"colors"`
	Seed   uint32   `json:"seed"`
}

type replicaLatencyMode struct {
	Mode string `json:"mode"`
	MS   int    `json:"ms"`
}

type replicaReceiver struct {
	Name     string `json:"name"`
	Model    string `json:"model"`
	Auth     string `json:"auth"`
	Online   bool   `json:"online"`
	Selected bool   `json:"selected"`
	Stereo   bool   `json:"stereo"`
}

type replicaPlayer struct {
	App     string        `json:"app"`
	Current int           `json:"current"`
	Items   []replicaItem `json:"items"`
}

type replicaItem struct {
	Title    string       `json:"title"`
	Artist   string       `json:"artist"`
	TitleEN  string       `json:"titleEn"`
	ArtistEN string       `json:"artistEn"`
	Duration int          `json:"duration"`
	Position int          `json:"position"`
	Colors   []string     `json:"colors"`
	Lamp     *replicaLamp `json:"lamp,omitempty"`
}

// replicaCorpus adds the tracks each player carries beyond the one the fixture
// happens to be showing. The page needs them: without a second and third item
// the transport's next has nothing to move through. Each item carries both
// languages, because the page switches copy without refetching the dataset;
// the first track's English values come from the app's own English fixture.
var replicaCorpus = map[string][]replicaItem{
	"Apple Music": {
		{Title: "落日公路", TitleEN: "Sunset Highway", Artist: "霓虹车队", ArtistEN: "Neon Convoy", Duration: 207},
		{Title: "玻璃海", TitleEN: "Glass Sea", Artist: "潮汐信号", ArtistEN: "Tide Signal", Duration: 312},
	},
	"Spotify": {
		{Title: "绯红", TitleEN: "Crimson", Artist: "玫瑰刺", ArtistEN: "Rose Thorn", Duration: 238},
		{Title: "雪松与雾", TitleEN: "Cedar and Fog", Artist: "苔原电台", ArtistEN: "Tundra Radio", Duration: 281},
	},
}

// replicaPalettes are the six poster palettes the page paints, one per demo
// track. The first is the shipped page palette, which the page's @property
// --amb-* initial values match; the dataset carries the rest so no track wraps
// onto another's artwork. The page reads colors and lamp blobs per track.
var replicaPalettes = [6][3]string{
	{"#5b5be0", "#8f6fd8", "#3d9be0"},
	{"#ff9a3d", "#ef5d7a", "#9a4fa8"},
	{"#35d0c5", "#3aa6d0", "#1b3d8f"},
	{"#e34b6f", "#c22a55", "#7a1f3d"},
	{"#2f6b4f", "#7fae5a", "#b08954"},
	{"#d8b25a", "#8a5a2b", "#4a2f1a"},
}

// replicaStreamingApp builds the one fixture the page projects: the base
// fixture with the catalog's streaming scenario applied. The catalog reads
// i18n.Locale, so the test locale moves with the requested language.
func replicaStreamingApp(t *testing.T, locale string) *App {
	t.Helper()
	old := testLocale
	defer func() { testLocale = old }()
	testLocale = locale
	a := Fixture(locale)
	for _, sc := range Catalog() {
		if sc.ID == "flyout.stream" {
			sc.Apply(a)
			return a
		}
	}
	t.Fatal("the catalog has no flyout.stream scenario")
	return nil
}

// replicaSessions builds the corpus: one list per player, the fixture's
// session first in its own list, the player it is showing first overall. Each
// item carries both languages and its own poster palette and lamp, so the page
// never has to guess either from the track's position.
func replicaSessions(a, en *App) []replicaPlayer {
	var players []replicaPlayer
	seen := map[string]bool{}
	for _, s := range a.nowSnap.Sessions {
		if seen[s.AppName] {
			continue
		}
		seen[s.AppName] = true
		titleEN, artistEN := s.Title, s.Artist
		if t, ar, ok := replicaEnglishTrack(en, s.AppName); ok {
			titleEN, artistEN = t, ar
		}
		items := []replicaItem{{
			Title:    s.Title,
			Artist:   s.Artist,
			TitleEN:  titleEN,
			ArtistEN: artistEN,
			Duration: int(s.Duration.Seconds()),
			Position: int(s.Position.Seconds()),
		}}
		items = append(items, replicaCorpus[s.AppName]...)
		players = append(players, replicaPlayer{App: s.AppName, Items: items})
	}
	for _, app := range []string{"Spotify"} {
		if !seen[app] {
			players = append(players, replicaPlayer{App: app, Items: replicaCorpus[app]})
		}
	}
	if i := a.nowMediaIndex(); i < len(a.nowSnap.Sessions) {
		current := a.nowSnap.Sessions[i].AppName
		for j := range players {
			if players[j].App == current && j > 0 {
				players[0], players[j] = players[j], players[0]
				break
			}
		}
	}
	track := 0
	for pi := range players {
		for ii := range players[pi].Items {
			palette := replicaPaletteFor(players[pi].Items[ii].Title)
			if palette < 0 {
				palette = track % len(replicaPalettes)
			}
			colors := replicaPalettes[palette]
			players[pi].Items[ii].Colors = []string{colors[0], colors[1], colors[2]}
			players[pi].Items[ii].Lamp = replicaCoverLamp(colors)
			track++
		}
	}
	return players
}

// replicaPaletteFor keeps each demo track's shipped poster palette: the five
// tracks the page carried before the dataset gained Chrome's session keep
// theirs, and Chrome's track takes the sixth. Falling back to flat position
// would move 绯红 to the green palette when a track is inserted before it.
func replicaPaletteFor(title string) int {
	switch title {
	case "夜航":
		return 0
	case "落日公路":
		return 1
	case "玻璃海":
		return 2
	case "绯红":
		return 3
	case "雪松与雾":
		return 4
	case "Open Water Sessions":
		return 5
	}
	return -1
}

// replicaEnglishTrack is the English fixture's version of a player's current
// track. The first track of every player comes from the app's own fixture in
// both languages; the corpus carries the translations of the rest.
func replicaEnglishTrack(a *App, app string) (title, artist string, ok bool) {
	for _, s := range a.nowSnap.Sessions {
		if s.AppName == app {
			return s.Title, s.Artist, true
		}
	}
	return "", "", false
}

// replicaCoverLamp derives a track's cover lamp through the app's own
// lamp.CoverPreset, so every track's preview follows its artwork and the blob
// scatter cannot drift from drawLamp.
func replicaCoverLamp(colors [3]string) *replicaLamp {
	p, ok := lamp.CoverPreset(replicaCoverImage(colors))
	if !ok {
		return nil
	}
	l := &replicaLamp{Cover: true, Base: hexColor(p.Base)}
	for _, b := range p.Blobs() {
		l.Blobs = append(l.Blobs, replicaBlob{
			X: b.X, Y: b.Y, R: b.Radius, Color: hexColor(b.Color), Alpha: b.Alpha,
		})
	}
	return l
}

// replicaCoverImage paints the three-color cover a track's poster shows, so
// CoverPreset sees the same palette the page does.
func replicaCoverImage(colors [3]string) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	c := [3]color.RGBA{replicaHex(colors[0]), replicaHex(colors[1]), replicaHex(colors[2])}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			k := float64(x+y) / 126
			var out color.RGBA
			if k < 0.5 {
				out = replicaBlend(c[0], c[1], k*2)
			} else {
				out = replicaBlend(c[1], c[2], (k-0.5)*2)
			}
			img.SetRGBA(x, y, out)
		}
	}
	return img
}

func replicaHex(s string) color.RGBA {
	n, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil {
		return color.RGBA{A: 0xff}
	}
	return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff}
}

func replicaBlend(a, b color.RGBA, k float64) color.RGBA {
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-k) + float64(y)*k + 0.5)
	}
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 0xff}
}

// replicaTemplates lists the six ambient templates in display order, with the
// names both catalogs give them.
func replicaTemplates() []replicaTemplate {
	var out []replicaTemplate
	for _, p := range lamp.All() {
		colors := make([]string, 0, len(p.Colors))
		for _, c := range p.Colors {
			colors = append(colors, hexColor(c))
		}
		out = append(out, replicaTemplate{
			ID:     p.ID,
			Name:   replicaLampPresetName("zh-CN", p.ID),
			NameEN: replicaLampPresetName("en", p.ID),
			Colors: colors,
			Seed:   p.Seed,
		})
	}
	return out
}

// replicaLampPresetName is the catalog's name for a template ID in one language.
func replicaLampPresetName(locale, id string) string {
	msgs := locales.In(locale)
	switch id {
	case "ember":
		return msgs.LampPresetEmber()
	case "dune":
		return msgs.LampPresetDune()
	case "moss":
		return msgs.LampPresetMoss()
	case "tide":
		return msgs.LampPresetTide()
	case "orchid":
		return msgs.LampPresetOrchid()
	case "moon":
		return msgs.LampPresetMoon()
	}
	return id
}

// replicaScenarios lists the catalog states the landing page projects.
func replicaScenarios() []string {
	var out []string
	for _, sc := range Catalog() {
		if sc.SiteDemo {
			out = append(out, sc.ID)
		}
	}
	sort.Strings(out)
	return out
}

// replicaPhase maps the app's one interface state to the page's vocabulary.
// The page draws no error face, so both idle and error read as idle there.
func replicaPhase(p phase) string {
	switch p {
	case phaseStreaming:
		return "streaming"
	case phaseConnecting:
		return "connecting"
	}
	return "idle"
}

// replicaDataValue builds the dataset the page loads. Both the Chinese and
// English fixtures are rendered, so every track carries the copy each language
// shows and the first track uses sampleTrack("en") rather than a translation
// kept in this test.
func replicaDataValue(t *testing.T) replicaData {
	t.Helper()
	old := testLocale
	defer func() { testLocale = old }()
	testLocale = "zh-CN"
	a := replicaStreamingApp(t, "zh-CN")
	en := replicaStreamingApp(t, "en")
	data := replicaData{
		Volume:    int(a.volume),
		Latency:   string(a.latencyMode),
		CustomMS:  int(a.latencyCustom),
		Templates: replicaTemplates(),
		Scenarios: replicaScenarios(),
		LatencyModes: []replicaLatencyMode{
			{Mode: string(settings.LatencyLow), MS: settings.LatencyLowMS},
			{Mode: string(settings.LatencyStandard), MS: settings.LatencyStandardMS},
			{Mode: string(settings.LatencyStable), MS: settings.LatencyStableMS},
		},
		State: replicaState{
			Phase:    replicaPhase(phaseOf(a.snap)),
			Standby:  a.snap.Standby,
			Applying: a.latencyApplying,
			Pairing:  a.snap.State == sender.StatePairing,
			PTP:      a.snap.PTPReady,
		},
		Light: replicaLight{Off: a.lampOff, Cover: a.lampCover, Preset: a.lampPreset},
	}
	for _, d := range a.snap.Devices {
		data.Receivers = append(data.Receivers, replicaReceiver{
			Name:     d.Name,
			Model:    d.Model,
			Auth:     authName(d.Auth),
			Online:   d.Online,
			Selected: d.ID == a.snap.SelectedID,
			Stereo:   d.IsStereoGroup(),
		})
	}
	data.Sessions = replicaSessions(a, en)
	if a.nowLampOK {
		l := &replicaLamp{Cover: a.lampCover, Base: hexColor(a.nowLamp.Base)}
		for _, b := range a.nowLamp.Blobs() {
			l.Blobs = append(l.Blobs, replicaBlob{
				X: b.X, Y: b.Y, R: b.Radius, Color: hexColor(b.Color), Alpha: b.Alpha,
			})
		}
		data.Lamp = l
	}
	return data
}

// replicaDataScript renders the dataset as the script the page loads.
func replicaDataScript(t *testing.T) string {
	t.Helper()
	data := replicaDataValue(t)
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return "/* Generated from internal/app's demo fixture by TestExportReplicaData.\n" +
		"   Edit the fixture, then refresh this file:\n" +
		"     SAUNDO_DEMO_OUT=../../site/assets/demo-data.js go test -tags preview -run TestExportReplicaData ./internal/app/\n" +
		"   TestReplicaDataIsCurrent fails while the two disagree. */\n" +
		"window.SAUNDO_DEMO = " + string(b) + ";\n"
}

// replicaDataFromFile reads the checked-in dataset, so the catalog gate tests
// the file the page loads rather than a value generated beside it.
func replicaDataFromFile(t *testing.T) replicaData {
	t.Helper()
	b, err := os.ReadFile(replicaDataPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		t.Fatalf("%s holds no dataset object", replicaDataPath)
	}
	var data replicaData
	if err := json.Unmarshal([]byte(s[start:end+1]), &data); err != nil {
		t.Fatalf("%s: %v", replicaDataPath, err)
	}
	return data
}

// TestReplicaDataIsCurrent fails when the page's dataset and the fixture that
// the app-side tests render have parted ways. Refresh it with
// SAUNDO_DEMO_OUT=<path> go test -tags preview -run TestExportReplicaData.
func TestReplicaDataIsCurrent(t *testing.T) {
	want := replicaDataScript(t)
	got, err := os.ReadFile(replicaDataPath)
	if err != nil {
		t.Fatalf("%v — write it with SAUNDO_DEMO_OUT=%s go test -tags preview -run TestExportReplicaData ./internal/app/", err, replicaDataPath)
	}
	// A Windows checkout may hold the file with CRLF line endings; the
	// generator writes LF, so compare the text rather than the bytes.
	if strings.ReplaceAll(string(got), "\r\n", "\n") != want {
		t.Fatalf("%s is stale: it no longer matches the app's demo fixture. Refresh it with\n"+
			"  SAUNDO_DEMO_OUT=%s go test -tags preview -run TestExportReplicaData ./internal/app/",
			replicaDataPath, replicaDataPath)
	}
}

// TestExportReplicaData writes the dataset the page loads.
func TestExportReplicaData(t *testing.T) {
	out := os.Getenv("SAUNDO_DEMO_OUT")
	if out == "" {
		t.Skip("set SAUNDO_DEMO_OUT to write the page's demo dataset")
	}
	if err := os.WriteFile(out, []byte(replicaDataScript(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
}

// hexColor is a colour the page can hand straight to CSS.
func hexColor(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// authName is the receiver's auth in the vocabulary the page localises. The
// device package owns the token; string(Auth) would put a NUL or a control
// character in the dataset for the values without a byte spelling.
func authName(a device.Auth) string {
	return a.String()
}

// TestAuthNameSerializesEveryAuth fails when an auth the engine can report
// reaches the page as a control character instead of a token.
func TestAuthNameSerializesEveryAuth(t *testing.T) {
	for a := device.AuthNone; a <= device.AuthHAPPassword; a++ {
		got := authName(a)
		if got != a.String() {
			t.Fatalf("authName(%d) = %q, want %q", a, got, a.String())
		}
		for _, r := range got {
			if r < 0x20 || r == 0x7f {
				t.Fatalf("authName(%d) = %q carries control character %U", a, got, r)
			}
		}
	}
}

// TestReplicaDataCarriesBothLanguages fails when the dataset carries a track
// in one language only: the English page would paint the Chinese title, and
// the cover lamp would name the wrong track.
func TestReplicaDataCarriesBothLanguages(t *testing.T) {
	data := replicaDataValue(t)
	firstEN := sampleTrack("en")
	if len(data.Sessions) == 0 || len(data.Sessions[0].Items) == 0 {
		t.Fatal("the dataset has no opening track")
	}
	first := data.Sessions[0].Items[0]
	if first.TitleEN != firstEN.Title || first.ArtistEN != firstEN.Artist {
		t.Fatalf("opening track English = %q / %q, want sampleTrack(\"en\") = %q / %q",
			first.TitleEN, first.ArtistEN, firstEN.Title, firstEN.Artist)
	}
	for _, p := range data.Sessions {
		for _, it := range p.Items {
			if it.Title == "" || it.TitleEN == "" || it.Artist == "" || it.ArtistEN == "" {
				t.Fatalf("%s / %q has a missing title or artist in one language: %+v", p.App, it.Title, it)
			}
		}
	}
}

// TestReplicaDataGivesEveryTrackItsOwnPalette fails when a track wraps onto
// another track's poster palette or carries no cover lamp, which is what made
// the cover-mode preview paint the previous track's artwork.
func TestReplicaDataGivesEveryTrackItsOwnPalette(t *testing.T) {
	data := replicaDataValue(t)
	seen := map[string]bool{}
	tracks := 0
	for _, p := range data.Sessions {
		for _, it := range p.Items {
			tracks++
			if len(it.Colors) != 3 {
				t.Fatalf("%s / %q has %d colors, want 3", p.App, it.Title, len(it.Colors))
			}
			key := strings.Join(it.Colors, ",")
			if seen[key] {
				t.Fatalf("%s / %q reuses palette %s", p.App, it.Title, key)
			}
			seen[key] = true
			if it.Lamp == nil || len(it.Lamp.Blobs) == 0 {
				t.Fatalf("%s / %q has no cover lamp", p.App, it.Title)
			}
		}
	}
	if tracks != len(replicaPalettes) {
		t.Fatalf("the corpus has %d tracks and %d palettes", tracks, len(replicaPalettes))
	}
}
