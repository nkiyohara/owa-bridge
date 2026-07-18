// Command normalizefiles gives a generated directory a deterministic mtime.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", "", "generated directory to normalize")
	timestamp := flag.String("time", "", "RFC3339 timestamp to apply")
	flag.Parse()

	if flag.NArg() != 0 {
		fatal(errors.New("positional arguments are not accepted"))
	}
	if err := validateRoot(*root); err != nil {
		fatal(err)
	}
	when, err := time.Parse(time.RFC3339, *timestamp)
	if err != nil {
		fatal(fmt.Errorf("parse --time as RFC3339: %w", err))
	}
	if err := normalize(*root, when); err != nil {
		fatal(err)
	}
}

func validateRoot(root string) error {
	if root == "" {
		return errors.New("--root is required")
	}
	clean := filepath.Clean(root)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("--root must be a specific relative directory below the working directory")
	}
	return nil
}

func normalize(root string, when time.Time) error {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink %q", path)
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("refusing non-regular file %q", path)
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("inspect %q: %w", root, err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("%q is empty", root)
	}

	// Update children before their parents so directory mtimes remain fixed.
	sort.Slice(paths, func(i, j int) bool {
		return len(paths[i]) > len(paths[j])
	})
	for _, path := range paths {
		if err := os.Chtimes(path, when, when); err != nil {
			return fmt.Errorf("set timestamp on %q: %w", path, err)
		}
	}
	return nil
}

func fatal(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "normalize files: %v\n", err)
	os.Exit(1)
}
