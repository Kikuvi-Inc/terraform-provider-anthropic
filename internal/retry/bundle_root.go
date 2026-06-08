// Copyright (c) Ippon
// SPDX-License-Identifier: MPL-2.0

package retry

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DeriveBundleRoot returns the common parent directory of filePaths and the
// directory's base name. It is the shared way Skill / SkillVersion callers
// compute the bundleRoot and dirName that MultipartUpload needs:
// the API requires a single top-level directory in the uploaded body, and
// dirName must match the `name` field of the bundle's SKILL.md frontmatter.
//
// Using the *common* parent (longest shared prefix at a path-segment
// boundary) — rather than `filepath.Dir(filePaths[0])` — keeps the result
// independent of input ordering. With `filepath.Dir(filePaths[0])`, callers
// that happened to receive paths whose first element was nested in a
// subdirectory (for example because `fileset()` returned its results in
// lexicographic order, and a top-level subdirectory whose name sorts before
// `SKILL.md` came first) would derive a sub-subdirectory as the bundle root
// and then fail every other file as "outside bundle root".
//
// Returns an error if filePaths is empty or if the paths share no common
// parent. The caller is expected to enforce non-empty filePaths separately
// (e.g. via a schema validator), but the empty check here keeps the helper
// safe to use in isolation.
func DeriveBundleRoot(filePaths []string) (root, dirName string, err error) {
	if len(filePaths) == 0 {
		return "", "", fmt.Errorf("DeriveBundleRoot: no file paths provided")
	}

	root = filepath.Dir(filePaths[0])
	for _, p := range filePaths[1:] {
		root = commonPathPrefix(root, filepath.Dir(p))
		if root == "" {
			return "", "", fmt.Errorf("DeriveBundleRoot: files do not share a common parent directory: %q vs %q", filePaths[0], p)
		}
	}

	dirName = filepath.Base(root)
	return root, dirName, nil
}

// commonPathPrefix returns the longest shared parent directory of a and b,
// trimmed at a path-segment boundary. The result preserves whatever
// separator style (forward or back slash) the inputs used. Empty input
// returns the other input. If the paths share no segments (e.g. different
// volumes on Windows, or one absolute / one relative), the result is the
// empty string.
func commonPathPrefix(a, b string) string {
	if a == b {
		return a
	}
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	// Normalise to OS-native separators so we compare segment-by-segment
	// consistently. filepath.Clean also strips trailing separators and
	// collapses repeated ones.
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	sep := string(filepath.Separator)
	aSegs := strings.Split(a, sep)
	bSegs := strings.Split(b, sep)
	n := len(aSegs)
	if len(bSegs) < n {
		n = len(bSegs)
	}
	common := 0
	for common < n && aSegs[common] == bSegs[common] {
		common++
	}
	if common == 0 {
		return ""
	}
	return strings.Join(aSegs[:common], sep)
}
