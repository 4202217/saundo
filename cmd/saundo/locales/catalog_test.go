package locales

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestCatalogKeyParity keeps the translations complete. mygo i18n check
// warns about missing messages; this test fails.
func TestCatalogKeyParity(t *testing.T) {
	en := readCatalog(t, "en.json")
	zh := readCatalog(t, "zh-CN.json")
	for key := range en {
		if _, ok := zh[key]; !ok {
			t.Errorf("zh-CN.json is missing %q", key)
		}
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("en.json has no %q", key)
		}
	}
}

// TestCatalogHasNoDeadKeys fails when a catalog key has no caller outside the
// generated accessors and the tests. Delete the key instead of shipping copy
// no surface can show.
func TestCatalogHasNoDeadKeys(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	var source strings.Builder
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".audit", "bin", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(root, "cmd/saundo/locales/catalog_gen.go")) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source.Write(body)
		source.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	src := source.String()
	for key := range readCatalog(t, "en.json") {
		accessor := strings.ToUpper(key[:1]) + key[1:]
		if !strings.Contains(src, "locales."+accessor+"(") {
			t.Errorf("catalog key %q has no caller (locales.%s)", key, accessor)
		}
	}
}

// TestCatalogBannedTerms keeps the terminology rules in docs/ui-copy.md.
// A banned term names an implementation detail or a second word for a
// concept the interface already names once.
func TestCatalogBannedTerms(t *testing.T) {
	banned := map[string][]string{
		"zh-CN": {
			"音画同步", "接收设备", "接收端", "推流", "串流", "参考实现",
			"握手", "退避", "auth-setup", "自定义模板", "更新窗口", "打开一台",
			"控制面板", "弱网", "断音", "未验证", "静默驻留",
			"让图标动起来", "设备列表", "当前设备", "未选择设备",
			"立即检查", "这台设备", "打开下方", "顶屏灯光",
			"浮窗", "显示面板", "和面板", "签名", "按 1、2、5",
			"调度缓冲", "接收器输出", "丢样本", "当前接收器", "熄灯",
		},
		"en": {
			"auth-setup", "Backoff", "Reference buffering", "update window",
			"Refresh devices", "No devices", "Device list", "this device",
			"Top display light", "Show panel", "tray flyout", "Flyout as popover",
			"the panel", "signature", "Retries at", "Current receiver",
		},
	}
	for loc, terms := range banned {
		catalog := readCatalog(t, loc+".json")
		for key, value := range catalog {
			for _, term := range terms {
				if strings.Contains(value, term) {
					t.Errorf("%s: %q contains banned term %q", loc, key, term)
				}
			}
		}
	}
}

// TestCatalogUniqueValues keeps a new key from quietly reusing another key's
// copy. The shared set names keys that intentionally say the same thing in
// different surfaces. A new duplicate fails so its author makes a decision.
func TestCatalogUniqueValues(t *testing.T) {
	shared := map[string]bool{
		"appName": true, "trayTooltip": true, "settingsAboutBrand": true,
		"trayDisconnect": true, "actionDisconnect": true,
		"flyoutDevices": true, "pinFallbackDevice": true, "settingsNavReceivers": true,
		"statusStreaming": true, "settingsNavNowPlaying": true,
		"flyoutNowPlayingSession": true,
		"flyoutPlayback":          true, "flyoutPlay": true,
		"actionCancel": true, "updateCancel": true,
		"actionRetry": true, "updateRetry": true,
		"actionConnect": true, "settingsNavConnection": true,
		"commonSettings": true, "receiverPasswordSet": true,
		"settingsCover": true, "lampPresetCover": true,
		"settingsSave": true, "receiverDialogSave": true,
		"languageSystem": true, "themeSystem": true,
		"receiverGroupHidden": true, "receiverChipHidden": true,
		"receiverAddTitle": true, "receiverDialogTitleAdd": true,
		"receiverPasswordTitle": true, "receiverAuthPassword": true, "receiverFieldPassword": true,
		"updateAvailableTitle": true, "flyoutUpdateAvailable": true,
		"updateAvailableMeta": true, "updateNotesMeta": true,
		"settingsNavLamp": true, "settingsLampCardTitle": true, "flyoutLampSection": true,
		"settingsLampTemplates": true, "lampSourceTemplate": true,
		"lampSourceCover": true, "summaryCurrentDevice": true,
		"passwordLabel": true, "receiverFieldAuth": true,
		"themeLight":            true,
		"settingsLatencyStable": true, "settingsChannelStable": true,
		// Both choices mean "none": no receiver authentication, no cover.
		"receiverAuthNone": true, "settingsCoverNone": true,
	}
	for _, loc := range []string{"zh-CN", "en"} {
		byValue := map[string][]string{}
		for key, value := range readCatalog(t, loc+".json") {
			byValue[value] = append(byValue[value], key)
		}
		for value, keys := range byValue {
			if len(keys) < 2 {
				continue
			}
			slices.Sort(keys)
			allowed := true
			for _, key := range keys {
				if !shared[key] {
					allowed = false
					break
				}
			}
			if !allowed {
				t.Errorf("%s: %q is shared by %v; use one key or add the keys to shared", loc, value, keys)
			}
		}
	}
}

func readCatalog(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		if strings.HasPrefix(key, "@") {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			t.Fatalf("%s: %q is not a string: %v", name, key, err)
		}
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s: %q is empty", name, key)
		}
		out[key] = text
	}
	return out
}

// TestRowHintsHaveNoTerminalPunctuation pins the punctuation rule: a
// settings-row subtitle and a link description are labels, not prose, so they
// carry no final period. Page descriptions, card body text, notes, dialog
// copy, and status lines keep sentence punctuation.
func TestRowHintsHaveNoTerminalPunctuation(t *testing.T) {
	keys := []string{
		"settingsSenderNameDesc",
		"settingsLaunchAtStartDesc",
		"settingsTrayAnimationDesc",
		"settingsLanguageDesc",
		"settingsThemeDesc",
		"settingsPlaybackControlsDesc",
		"settingsCoverColorsDesc",
		"settingsAudioLevelDesc",
		"settingsCoverDesc",
		"settingsAutoUpdateDesc",
		"settingsAutoDownloadDesc",
		"settingsChannelDesc",
		"settingsReconnectDesc",
		"settingsStrictAuthDesc",
		"settingsMuteLocalDesc",
		"settingsMuteLocalDescOn",
		"settingsMuteLocalDescWaiting",
		"settingsMuteLocalDescFailed",
		"settingsAudioFormatDesc",
		"settingsSyncModeDesc",
		"settingsSystemOutputDesc",
		"settingsAppCaptureDesc",
		"settingsCaptureStatusDesc",
		"receiverAutoToggleDesc",
		"receiverDelayDesc",
		"lampPriorityDesc",
		"aboutHomeDesc",
		"aboutReleasesDesc",
		"aboutIssuesDesc",
		"aboutLogDesc",
		"aboutLicenseDesc",
		"aboutNoticesDesc",
	}
	for _, loc := range []string{"en", "zh-CN"} {
		catalog := readCatalog(t, loc+".json")
		for _, key := range keys {
			value, ok := catalog[key]
			if !ok {
				t.Errorf("%s.json is missing %q", loc, key)
				continue
			}
			if strings.HasSuffix(value, ".") || strings.HasSuffix(value, "。") {
				t.Errorf("%s.json %q ends with a period: %q", loc, key, value)
			}
		}
	}
}

// TestVolumeValueRoundsToWholePercent keeps the receiver's fractional
// feedback out of the readout.
func TestVolumeValueRoundsToWholePercent(t *testing.T) {
	for _, locale := range []string{"en", "zh-CN"} {
		for _, c := range []struct {
			v    float64
			want string
		}{{22.034, "22%"}, {65, "65%"}, {99.6, "100%"}} {
			got := Catalog.Format(locale, "flyoutVolumeValue", map[string]any{"v": c.v})
			if got != c.want {
				t.Errorf("%s: volume %v = %q, want %q", locale, c.v, got, c.want)
			}
		}
	}
}
