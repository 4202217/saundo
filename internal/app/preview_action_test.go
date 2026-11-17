//go:build preview

package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEveryActionHasAHandler(t *testing.T) {
	for _, action := range allActions {
		if previewHandlers[action] == nil {
			t.Errorf("%s: no preview handler", action)
		}
	}
}

func TestEveryHandlerIsAKnownAction(t *testing.T) {
	known := make(map[Action]bool, len(allActions))
	for _, action := range allActions {
		known[action] = true
	}
	for action := range previewHandlers {
		if !known[action] {
			t.Errorf("%s: handler has no allActions entry", action)
		}
	}
}

// TestMissingHandlerPanics seeds a missing handler: act must panic with the
// action name and must not run the real closure.
func TestMissingHandlerPanics(t *testing.T) {
	a := Fixture(previewLocale)
	h, ok := previewHandlers[ActionVolume]
	if !ok {
		t.Fatal("ActionVolume has no handler to remove")
	}
	delete(previewHandlers, ActionVolume)
	t.Cleanup(func() { previewHandlers[ActionVolume] = h })
	ran := false
	mustPanicWith(t, string(ActionVolume), func() {
		a.act(ActionVolume, 50.0, func() { ran = true })
	})
	if ran {
		t.Fatal("act ran the real closure")
	}
}

// TestUnknownActionPanics proves an action outside allActions fails closed.
func TestUnknownActionPanics(t *testing.T) {
	ran := false
	mustPanicWith(t, "preview.unknown", func() {
		Fixture(previewLocale).act(Action("preview.unknown"), nil, func() { ran = true })
	})
	if ran {
		t.Fatal("act ran the real closure")
	}
}

// TestWrongArgumentPanics proves a handler that rejects its argument fails
// closed instead of falling through.
func TestWrongArgumentPanics(t *testing.T) {
	ran := false
	mustPanicWith(t, string(ActionVolume), func() {
		Fixture(previewLocale).act(ActionVolume, "loud", func() { ran = true })
	})
	if ran {
		t.Fatal("act ran the real closure")
	}
}

// TestActionCallSitesAreRegistered scans every view file for act calls and
// requires each first argument to be a typed constant in allActions with a
// handler. A new call site that forgets the list fails here.
func TestActionCallSitesAreRegistered(t *testing.T) {
	consts := actionConstants(t)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	fset := token.NewFileSet()
	for _, path := range files {
		// The tests pass a non-constant action on purpose, to prove the panic.
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "act" {
				return true
			}
			calls++
			pos := fset.Position(call.Pos())
			if len(call.Args) == 0 {
				t.Errorf("%s: act has no arguments", pos)
				return true
			}
			id, ok := call.Args[0].(*ast.Ident)
			if !ok {
				t.Errorf("%s: act's first argument is not an action constant", pos)
				return true
			}
			action, ok := consts[id.Name]
			if !ok {
				t.Errorf("%s: %s is not a declared Action constant", pos, id.Name)
				return true
			}
			if previewHandlers[action] == nil {
				t.Errorf("%s: %s has no preview handler", pos, id.Name)
			}
			return true
		})
	}
	if calls == 0 {
		t.Fatal("no act call sites were found")
	}
}

// actionConstants maps each constant name declared in actions.go to its
// Action value, so the call-site scan can compare identifiers.
func actionConstants(t *testing.T) map[string]Action {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "actions.go", nil, 0)
	if err != nil {
		t.Fatalf("actions.go: %v", err)
	}
	out := map[string]Action{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				out[name.Name] = Action(val)
			}
		}
	}
	if len(out) != len(allActions) {
		t.Fatalf("actions.go declares %d Action constants, allActions has %d", len(out), len(allActions))
	}
	return out
}

// TestUpdateChipOpensTheUpdatePage pins the action argument: the footer chip
// and the gear open different pages.
func TestUpdateChipOpensTheUpdatePage(t *testing.T) {
	old := previewShowHost
	t.Cleanup(func() { previewShowHost = old })
	var hosts []Surface
	previewShowHost = func(_ *App, host Surface) { hosts = append(hosts, host) }

	a := Fixture(previewLocale)
	a.page = pageGeneral
	a.actUpdateSettings()
	if a.page != pageUpdate {
		t.Fatalf("page = %d, want pageUpdate", a.page)
	}
	if len(hosts) != 1 || hosts[0] != SurfaceSettings {
		t.Fatalf("hosts = %v, want one settings window", hosts)
	}
}

// TestConnectAndDisconnectClearLatencyApply pins the reset the real connect
// and disconnect actions perform before they change the session.
func TestConnectAndDisconnectClearLatencyApply(t *testing.T) {
	oldAfter, oldOnMain := previewAfter, previewOnMain
	t.Cleanup(func() { previewAfter, previewOnMain = oldAfter, oldOnMain })
	previewAfter = func(time.Duration, func()) *time.Timer { return nil }
	previewOnMain = func(fn func()) { fn() }

	a := Fixture(previewLocale)
	applyLatencyApplying(a)
	if !a.previewAction(ActionConnect, "AA:BB:CC:DD:EE:02") {
		t.Fatal("connect was not handled")
	}
	if a.latencyApplying {
		t.Fatal("connect left the latency apply spinner")
	}
	applyLatencyApplying(a)
	if !a.previewAction(ActionDisconnect, nil) {
		t.Fatal("disconnect was not handled")
	}
	if a.latencyApplying {
		t.Fatal("disconnect left the latency apply spinner")
	}
}

// mustPanicWith fails unless fn panics with a value containing want.
func mustPanicWith(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if s, ok := r.(string); !ok || !strings.Contains(s, want) {
			t.Fatalf("panic = %v, want %q", r, want)
		}
	}()
	fn()
}
