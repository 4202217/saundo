//go:build preview

package app

// The landing page's demo dataset, generated from the same fixture and catalog
// the preview matrix renders. Ported from the render-demo research branch at
// f3f6bf8 and rebuilt on Fixture and the catalog.

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"sort"
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
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	NameEN string        `json:"nameEn"`
	Colors []string      `json:"colors"`
	Seed   uint32        `json:"seed"`
	Base   string        `json:"base"`
	Blobs  []replicaBlob `json:"blobs"`
}

type replicaLatencyMode struct {
	Mode string `json:"mode"`
	MS   int    `json:"ms"`
}

type replicaReceiver struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	Auth       string `json:"auth"`
	Online     bool   `json:"online"`
	Selected   bool   `json:"selected"`
	Stereo     bool   `json:"stereo"`
	Subtitle   string `json:"subtitle"`
	SubtitleEN string `json:"subtitleEn"`
}

type replicaPlayer struct {
	App     string        `json:"app"`
	Icon    string        `json:"icon,omitempty"`
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
	Cover    string       `json:"cover,omitempty"`
	Lamp     *replicaLamp `json:"lamp,omitempty"`
}

// replicaCorpus adds the tracks each player carries beyond the one the fixture
// happens to be showing. The page needs them: without a second and third item
// the transport's next has nothing to move through. Each item carries both
// languages, because the page switches copy without refetching the dataset;
// the first track's English values come from the app's own English fixture.
var replicaCorpus = map[string][]replicaItem{
	"Longwave": {
		{Title: "落日公路", TitleEN: "Sunset Highway", Artist: "霓虹车队", ArtistEN: "Neon Convoy", Duration: 207},
		{Title: "玻璃海", TitleEN: "Glass Sea", Artist: "潮汐信号", ArtistEN: "Tide Signal", Duration: 312},
	},
	"Cinder": {
		{Title: "绯红", TitleEN: "Crimson", Artist: "玫瑰刺", ArtistEN: "Rose Thorn", Duration: 238},
		{Title: "雪松与雾", TitleEN: "Cedar and Fog", Artist: "苔原电台", ArtistEN: "Tundra Radio", Duration: 281},
	},
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
func replicaSessions(a, en *App, art replicaArt) []replicaPlayer {
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
		players = append(players, replicaPlayer{App: s.AppName, Icon: art.Marks[s.AppName], Items: items})
	}
	for _, app := range []string{"Cinder"} {
		if !seen[app] {
			players = append(players, replicaPlayer{App: app, Icon: art.Marks[app], Items: replicaCorpus[app]})
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
	for pi := range players {
		for ii := range players[pi].Items {
			it := &players[pi].Items[ii]
			cover := art.Covers[replicaArtCoverKey(it.Title)]
			it.Cover = cover.Cover
			it.Lamp, it.Colors = cover.Lamp, cover.Colors
		}
	}
	return players
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

// replicaTemplates lists the six ambient templates in display order, with the
// names both catalogs give them.
func replicaTemplates(art replicaArt) []replicaTemplate {
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
			Base:   art.Templates[p.ID].Base,
			Blobs:  art.Templates[p.ID].Blobs,
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

// replicaSubtitles renders each device's line through the app's own
// deviceSubtitle, so the page shows the same model, auth, and stereo wording
// the flyout paints without re-deriving it from parts.
func replicaSubtitles(devices []device.Device, locale string) map[string]string {
	old := testLocale
	defer func() { testLocale = old }()
	testLocale = locale
	out := make(map[string]string, len(devices))
	for _, d := range devices {
		out[d.ID] = deviceSubtitle(d)
	}
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
	art := loadReplicaArt(t)
	data := replicaData{
		Volume:    int(a.volume),
		Latency:   string(a.latencyMode),
		CustomMS:  int(a.latencyCustom),
		Templates: replicaTemplates(art),
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
	subs := replicaSubtitles(a.snap.Devices, "zh-CN")
	enSubs := replicaSubtitles(en.snap.Devices, "en")
	for _, d := range a.snap.Devices {
		data.Receivers = append(data.Receivers, replicaReceiver{
			Name:       d.Name,
			Model:      d.Model,
			Auth:       authName(d.Auth),
			Online:     d.Online,
			Selected:   d.ID == a.snap.SelectedID,
			Stereo:     d.IsStereoGroup(),
			Subtitle:   subs[d.ID],
			SubtitleEN: enSubs[d.ID],
		})
	}
	data.Sessions = replicaSessions(a, en, art)
	if a.nowLampOK && len(data.Sessions) > 0 && len(data.Sessions[0].Items) > 0 {
		// The session lamp follows the opening track's cover, so it is pinned
		// with that cover rather than recomputed from its float-derived pixels.
		key := replicaArtCoverKey(data.Sessions[0].Items[0].Title)
		data.Lamp = art.Covers[key].Lamp
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
	gotText := strings.ReplaceAll(string(got), "\r\n", "\n")
	if gotText != want {
		t.Fatalf("%s is stale: it no longer matches the app's demo fixture. First drift:\n%s\n"+
			"  Refresh it with SAUNDO_DEMO_OUT=%s go test -tags preview -run TestExportReplicaData ./internal/app/",
			replicaDataPath, firstDrift(want, gotText), replicaDataPath)
	}
}

// firstDrift names the first lines that disagree, so a stale dataset or a
// cross-arch render difference names its field instead of forcing a blind
// regeneration.
func firstDrift(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) && i < len(g); i++ {
		if w[i] != g[i] {
			return fmt.Sprintf("  line %d: fixture %q, disk %q", i+1, w[i], g[i])
		}
	}
	if len(w) != len(g) {
		return fmt.Sprintf("  fixture holds %d lines, disk holds %d", len(w), len(g))
	}
	return "  none found line-by-line; the bytes differ only outside the text"
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

// TestReplicaDataGivesEveryTrackItsOwnCover fails when a track reuses another
// track's artwork or carries no cover lamp. The page paints the embedded
// cover and the lamp derived from it, so a shared image would make a media
// switch look like the light did not change.
func TestReplicaDataGivesEveryTrackItsOwnCover(t *testing.T) {
	data := replicaDataValue(t)
	seenCover := map[string]bool{}
	seenColors := map[string]bool{}
	tracks := 0
	for _, p := range data.Sessions {
		for _, it := range p.Items {
			tracks++
			if !strings.HasPrefix(it.Cover, "data:image/jpeg;base64,") {
				t.Fatalf("%s / %q has no embedded cover", p.App, it.Title)
			}
			if seenCover[it.Cover] {
				t.Fatalf("%s / %q reuses another track's cover", p.App, it.Title)
			}
			seenCover[it.Cover] = true
			if len(it.Colors) != 3 {
				t.Fatalf("%s / %q has %d colors, want 3", p.App, it.Title, len(it.Colors))
			}
			key := strings.Join(it.Colors, ",")
			if seenColors[key] {
				t.Fatalf("%s / %q reuses cover colors %s", p.App, it.Title, key)
			}
			seenColors[key] = true
			if it.Lamp == nil || len(it.Lamp.Blobs) == 0 {
				t.Fatalf("%s / %q has no cover lamp", p.App, it.Title)
			}
		}
	}
	if tracks != len(coverArtTitles()) {
		t.Fatalf("the corpus has %d tracks and %d covers", tracks, len(coverArtTitles()))
	}
}

// TestReplicaDataCarriesEveryPlayerMark fails when a session reaches the card
// with no icon, which leaves a player name beside a generic glyph.
func TestReplicaDataCarriesEveryPlayerMark(t *testing.T) {
	data := replicaDataValue(t)
	seen := map[string]bool{}
	for _, p := range data.Sessions {
		if !strings.HasPrefix(p.Icon, "data:image/png;base64,") {
			t.Fatalf("%s has no embedded mark", p.App)
		}
		if seen[p.Icon] {
			t.Fatalf("%s reuses another player's mark", p.App)
		}
		seen[p.Icon] = true
	}
}

// TestReplicaTemplatesCarryTheAppField fails when a template reaches the page
// without the base and six blobs the app's drawLamp paints, which is what let
// the page draw its own approximate swatch.
func TestReplicaTemplatesCarryTheAppField(t *testing.T) {
	data := replicaDataValue(t)
	if len(data.Templates) != len(lamp.All()) {
		t.Fatalf("the dataset has %d templates, want %d", len(data.Templates), len(lamp.All()))
	}
	for _, tpl := range data.Templates {
		if tpl.Base == "" || len(tpl.Blobs) != 6 {
			t.Fatalf("%s carries base %q and %d blobs, want a base and 6 blobs", tpl.ID, tpl.Base, len(tpl.Blobs))
		}
	}
}
