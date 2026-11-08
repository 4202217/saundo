package main

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildTargetProducesGUIBinary guards the double-click experience. A
// console-subsystem binary makes Windows open a blank terminal window beside
// the app. The build goes through mygo, which links -H=windowsgui and embeds
// resources/icon.png; a plain go build would lose both.
func TestBuildTargetProducesGUIBinary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "mygo build") || !strings.Contains(text, "-platform windows/amd64") {
		t.Fatal("the build target must use mygo build -platform windows/amd64 so the exe is a GUI binary with its icon")
	}
}

// TestBuildTargetProducesDarwinApp guards the macOS build branch: the .app
// bundle is what TCC attributes the audio-capture permission to, so a build
// that produced a bare binary would lose the capture prompt and the tray.
func TestBuildTargetProducesDarwinApp(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "-platform darwin/arm64") || !strings.Contains(text, "bin/saundo.app") {
		t.Fatal("the build target must produce bin/saundo.app via mygo build -platform darwin/arm64")
	}
	if !strings.Contains(text, "build-adapter.sh") || !strings.Contains(text, "SAUNDO_MAC_SIGN_IDENTITY") {
		t.Fatal("the darwin build must stage the adapter and take signing from SAUNDO_MAC_SIGN_IDENTITY")
	}
}

// TestMyGoConfigHasNoPlatformBlindResources guards the Windows build on a
// clean checkout. A resource outside a platform directory is required by
// every platform's mygo build, but the MediaRemote adapter exists only after
// a macOS build. A committed signing identity names one developer's
// certificate and fails codesign on CI and other Macs.
func TestMyGoConfigHasNoPlatformBlindResources(t *testing.T) {
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Resources []string `json:"resources"`
		MacOS     struct {
			SigningIdentity string `json:"signingIdentity"`
		} `json:"macos"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Resources) != 0 {
		t.Fatalf("mygo.json resources %v are required by every platform; ship them from cmd/saundo/resources/<goos>-<goarch>/ instead", cfg.Resources)
	}
	if cfg.MacOS.SigningIdentity != "" {
		t.Fatalf("mygo.json commits signing identity %q; pass -sign or SAUNDO_MAC_SIGN_IDENTITY instead", cfg.MacOS.SigningIdentity)
	}
}

// TestMyGoIconPathExists guards the embedded app icon. mygo resolves a
// relative icon path against the directory of mygo.json, and this project
// keeps resources/ at the repository root, not under cmd/saundo.
func TestMyGoIconPathExists(t *testing.T) {
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Icon string `json:"icon"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Icon == "" {
		cfg.Icon = filepath.Join("resources", "icon.png")
	}
	icon, err := filepath.Abs(cfg.Icon)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(icon)
	if err != nil {
		t.Fatalf("mygo icon %q does not exist: %v", icon, err)
	}
	defer f.Close()
	img, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatalf("mygo icon %q is not a PNG: %v", icon, err)
	}
	if img.Width != img.Height || img.Width < 256 {
		t.Fatalf("mygo icon %q is %dx%d, want a square PNG at least 256 px", icon, img.Width, img.Height)
	}
}
