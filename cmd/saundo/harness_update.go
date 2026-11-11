//go:build updateharness

// The update harness source: a test build reads the signed darwin manifest
// and its archive from a local directory instead of the network. The app
// still checks the archive against the public key baked into it, so the
// source can only install an update signed with that key.
package main

import (
	"context"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
)

func init() {
	dir := os.Getenv("SAUNDO_UPDATE_HARNESS")
	if dir == "" {
		return
	}
	mygo.Updater.SetSource(harnessSource{dir: dir})
}

type harnessSource struct{ dir string }

func (s harnessSource) Manifest(context.Context, bool) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.dir, "update-"+runtime.GOOS+"-"+runtime.GOARCH+".json"))
}

func (s harnessSource) Open(_ context.Context, raw string) (io.ReadCloser, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return os.Open(filepath.Join(s.dir, path.Base(u.Path)))
}
