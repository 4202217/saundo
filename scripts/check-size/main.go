// Command check-size keeps the codebase from growing more megaliths. Files
// above the 800-line cap when the ratchet landed are grandfathered at their
// current size; a new non-test, non-generated file may not cross the cap, and
// a grandfathered file may not grow until its next seam is extracted.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	lineCap      = 800
	baselinePath = "scripts/check-size/baseline.txt"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-size:", err)
		os.Exit(2)
	}
	if err := run(root); err != nil {
		fmt.Fprintln(os.Stderr, "check-size:", err)
		os.Exit(1)
	}
}

// run reads the baseline first. A missing or unreadable baseline is a setup
// error, not an empty one: otherwise deleting the file would turn the gate
// green.
func run(root string) error {
	base, err := loadBaseline(filepath.Join(root, baselinePath))
	if err != nil {
		return err
	}
	var violations []string
	seen := map[string]bool{}
	checked := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if generated(data) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		lines := countLines(data)
		checked++
		if max, ok := base[rel]; ok {
			seen[rel] = true
			switch {
			case lines > max:
				violations = append(violations, fmt.Sprintf(
					"%s has %d lines, above its %d-line baseline; extract the next seam instead of growing it",
					rel, lines, max))
			case lines < max:
				violations = append(violations, fmt.Sprintf(
					"%s has %d lines, below its %d-line baseline; lower the baseline so the ceiling ratchets down",
					rel, lines, max))
			}
			return nil
		}
		if lines > lineCap {
			violations = append(violations, fmt.Sprintf(
				"%s has %d lines, above the %d-line cap for a new file",
				rel, lines, lineCap))
		}
		return nil
	})
	if err != nil {
		return err
	}
	for rel := range base {
		if !seen[rel] {
			violations = append(violations, fmt.Sprintf(
				"baseline names %s, which no longer exists; remove the entry", rel))
		}
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		return fmt.Errorf("line budget exceeded:\n  %s", strings.Join(violations, "\n  "))
	}
	fmt.Printf("check-size: %d files within their line budget\n", checked)
	return nil
}

func loadBaseline(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	defer f.Close()
	out := map[string]int{}
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 2 {
			return nil, fmt.Errorf("baseline line %d: %q, want path<TAB>max", line, text)
		}
		max, err := strconv.Atoi(parts[1])
		if err != nil || max <= 0 {
			return nil, fmt.Errorf("baseline line %d: bad max %q", line, parts[1])
		}
		out[filepath.ToSlash(parts[0])] = max
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func countLines(data []byte) int {
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// generated skips the mygo catalog, whose size follows the translation
// catalogs rather than the code structure.
func generated(data []byte) bool {
	head := data
	if len(head) > 2048 {
		head = head[:2048]
	}
	return bytes.Contains(head, []byte("Code generated")) &&
		bytes.Contains(head, []byte("DO NOT EDIT"))
}
