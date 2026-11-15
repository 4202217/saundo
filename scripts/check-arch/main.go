// Command check-arch turns the architecture rules in AGENTS.md into a gate.
// It parses the tree once and reports every violated import seam and the
// concurrency shapes it can see: a go statement or time.AfterFunc in a listed
// sans-I/O package, time.After or time.Tick anywhere in product code, and a
// data channel made with a literal channel type and no capacity. A struct{}
// signal channel is allowed, and a channel type declared in another file is
// beyond what a parser-only check can see.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type importRule struct {
	path  string
	allow []string
	why   string
}

var importRules = []importRule{
	{
		path:  "github.com/egoist/mygo",
		allow: []string{"internal/app/", "cmd/saundo/"},
		why:   "the native UI toolkit belongs to the UI layer",
	},
	{
		path:  "saundo/internal/app",
		allow: []string{"cmd/saundo/"},
		why:   "the app package is a leaf",
	},
	{
		path:  "saundo/internal/engine",
		allow: []string{"internal/app/", "cmd/saundo/"},
		why:   "the engine is UI-agnostic and lower layers do not import it",
	},
}

// sansIOPackages own no sockets, timers, or threads. The host pushes events
// into them; a goroutine here would escape the single-owner event loop.
var sansIOPackages = []string{
	"internal/airplay/sender/",
	"internal/airplay/crypto/",
	"internal/airplay/alac/",
	"internal/airplay/device/",
	"internal/audio/",
}

// timerCalls name the standard-library helpers that leave a timer or ticker
// running until it fires, with no handle to stop it early.
var timerCalls = map[string]string{
	"After": "time.After cannot be stopped; use time.NewTimer and Stop it",
	"Tick":  "time.Tick leaks its ticker; use time.NewTicker and Stop it",
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-arch:", err)
		os.Exit(2)
	}
	if err := run(root); err != nil {
		fmt.Fprintln(os.Stderr, "check-arch:", err)
		os.Exit(1)
	}
}

// run checks every Go file under root and returns the violations together, so
// one pass reports the whole seam break instead of the first line.
func run(root string) error {
	var violations []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
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
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		violations = append(violations, checkImports(rel, f)...)
		if isProduct(rel) && !strings.HasSuffix(rel, "_test.go") {
			violations = append(violations,
				checkGoroutines(rel, fset, f)...)
			violations = append(violations,
				checkTimers(rel, fset, f)...)
			violations = append(violations,
				checkChannels(rel, fset, f)...)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		return fmt.Errorf("architecture seams broke:\n  %s", strings.Join(violations, "\n  "))
	}
	fmt.Println("check-arch: architecture seams hold")
	return nil
}

func checkImports(rel string, f *ast.File) []string {
	var out []string
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		for _, rule := range importRules {
			if p != rule.path && !strings.HasPrefix(p, rule.path+"/") {
				continue
			}
			if underAny(rel, rule.allow) {
				continue
			}
			out = append(out, fmt.Sprintf("%s imports %s; %s", rel, p, rule.why))
		}
	}
	return out
}

func underAny(rel string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func isProduct(rel string) bool {
	return strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "cmd/")
}

func checkGoroutines(rel string, fset *token.FileSet, f *ast.File) []string {
	if !underAny(rel, sansIOPackages) {
		return nil
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		g, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		out = append(out, fmt.Sprintf(
			"%s starts a goroutine; this package is sans-I/O and owns no threads",
			at(rel, fset, g.Pos())))
		return true
	})
	return out
}

func checkTimers(rel string, fset *token.FileSet, f *ast.File) []string {
	names := timeNames(f)
	if len(names) == 0 {
		return nil
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || !names[id.Name] {
			return true
		}
		if sel.Sel.Name == "AfterFunc" && underAny(rel, sansIOPackages) {
			out = append(out, fmt.Sprintf(
				"%s calls %s.AfterFunc; it starts a goroutine, and this package is sans-I/O",
				at(rel, fset, call.Pos()), id.Name))
			return true
		}
		why, banned := timerCalls[sel.Sel.Name]
		if !banned {
			return true
		}
		out = append(out, fmt.Sprintf("%s calls %s.%s; %s",
			at(rel, fset, call.Pos()), id.Name, sel.Sel.Name, why))
		return true
	})
	return out
}

func timeNames(f *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != "time" {
			continue
		}
		name := "time"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			names[name] = true
		}
	}
	return names
}

// checkChannels requires an explicit capacity on a data channel made with a
// literal channel type. A struct{} channel is a signal with no payload, so an
// unbuffered one is the intended shape. A channel type declared in another
// file remains a blind spot for a parser-only check.
func checkChannels(rel string, fset *token.FileSet, f *ast.File) []string {
	aliases := channelTypeNames(f)
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "make" {
			return true
		}
		if ch, ok := call.Args[0].(*ast.ChanType); ok {
			if !emptyStruct(ch.Value) {
				out = append(out, fmt.Sprintf(
					"%s makes a data channel with no capacity; declare one, or use a struct{} signal channel",
					at(rel, fset, call.Pos())))
			}
			return true
		}
		if alias, ok := call.Args[0].(*ast.Ident); ok && aliases[alias.Name] {
			out = append(out, fmt.Sprintf(
				"%s makes a channel through the type alias %s; write make(chan T, n) so the gate can see the capacity",
				at(rel, fset, call.Pos()), alias.Name))
		}
		return true
	})
	return out
}

func channelTypeNames(f *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, ok := ts.Type.(*ast.ChanType); ok {
				names[ts.Name.Name] = true
			}
		}
	}
	return names
}

func emptyStruct(e ast.Expr) bool {
	st, ok := e.(*ast.StructType)
	return ok && (st.Fields == nil || len(st.Fields.List) == 0)
}

func at(rel string, fset *token.FileSet, pos token.Pos) string {
	return fmt.Sprintf("%s:%d", rel, fset.Position(pos).Line)
}
