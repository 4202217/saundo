// Command check-platforms builds and vets the tree for both targets on any
// host, then rejects OS-specific imports in files the other build compiles.
// A change that compiles on the host but breaks the other platform fails
// here, locally, before CI. `make platforms` and `make check` run it.
package main

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type target struct {
	goos   string
	goarch string
}

var targets = []target{
	{"windows", "amd64"},
	{"darwin", "arm64"},
}

// darwinOnlyImports may appear only in files the windows build does not
// compile. A shared file importing them would drag the platform internals
// across the seam.
var darwinOnlyImports = []string{
	"saundo/internal/darwin",
	"github.com/ebitengine/purego",
}

var windowsOnlyImports = []string{
	"golang.org/x/sys/windows",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "check-platforms:", err)
		os.Exit(1)
	}
}

// goBinary honors the Makefile's GO override, so the gate works on a host
// where go is not on PATH.
func goBinary() string {
	if v := os.Getenv("GO"); v != "" {
		return v
	}
	return "go"
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := checkBuilds(root); err != nil {
		return err
	}
	return checkImports(root)
}

func checkBuilds(root string) error {
	for _, t := range targets {
		for _, args := range [][]string{{"build", "./..."}, {"vet", "./..."}} {
			cmd := exec.Command(goBinary(), args...)
			cmd.Dir = root
			cmd.Env = envFor(t)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("%s/%s go %s: %w", t.goos, t.goarch, args[0], err)
			}
		}
		fmt.Printf("check-platforms: %s/%s builds and vets\n", t.goos, t.goarch)
	}
	return nil
}

func envFor(t target) []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+3)
	for _, e := range env {
		if strings.HasPrefix(e, "GOOS=") || strings.HasPrefix(e, "GOARCH=") || strings.HasPrefix(e, "CGO_ENABLED=") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "GOOS="+t.goos, "GOARCH="+t.goarch, "CGO_ENABLED=0")
}

func checkImports(root string) error {
	ctxs := map[string]build.Context{}
	for _, t := range targets {
		ctx := build.Default
		ctx.GOOS, ctx.GOARCH = t.goos, t.goarch
		ctx.CgoEnabled = false
		ctxs[t.goos] = ctx
	}

	windowsCtx, darwinCtx := ctxs["windows"], ctxs["darwin"]
	var bad []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Match the go tool's own walk: dot and underscore directories,
			// testdata, and build output are not part of the tree it builds.
			switch {
			case strings.HasPrefix(d.Name(), "."), strings.HasPrefix(d.Name(), "_"):
				return filepath.SkipDir
			case d.Name() == "bin", d.Name() == "build", d.Name() == "node_modules", d.Name() == "testdata", d.Name() == "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		inWindows, err := windowsCtx.MatchFile(filepath.Dir(path), d.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		inDarwin, err := darwinCtx.MatchFile(filepath.Dir(path), d.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if inWindows && contains(darwinOnlyImports, p) {
				bad = append(bad, fmt.Sprintf("%s imports %s but the windows build compiles it", rel, p))
			}
			if inDarwin && contains(windowsOnlyImports, p) {
				bad = append(bad, fmt.Sprintf("%s imports %s but the darwin build compiles it", rel, p))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		return fmt.Errorf("platform imports leak across the seams:\n  %s", strings.Join(bad, "\n  "))
	}
	fmt.Println("check-platforms: platform imports stay behind their build tags")
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
