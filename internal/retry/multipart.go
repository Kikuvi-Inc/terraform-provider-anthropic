// Copyright (c) Ippon
// SPDX-License-Identifier: MPL-2.0

// Package retry provides retry helpers for operations that the Anthropic SDK
// cannot retry automatically.
//
// Multipart file uploads set req.Body without req.GetBody, so the SDK's
// built-in retry logic (which requires a replayable body) is bypassed for
// all 5xx responses. MultipartUpload works around this by re-opening files
// from disk on each attempt.
package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// NamedReader wraps an io.Reader with an explicit multipart filename.
//
// The Anthropic SDK encoder picks the multipart filename via this
// preference order: `interface{ Filename() string }`, then
// `interface{ Name() string }` (where `path.Base` is applied to the
// result), then the struct field name. We want the full bundle-relative
// name (e.g. `myskill/references/template.md`), so we deliberately:
//
//   - embed `io.Reader` rather than `*os.File`, which prevents the
//     concrete file's `Name()` method (= absolute on-disk path, which
//     would then be `path.Base`'d to a flat basename) from being
//     promoted onto NamedReader;
//   - provide `Filename()` explicitly so the SDK reaches it first.
//
// The compile-time assertion below locks that contract.
type NamedReader struct {
	io.Reader
	name string
}

// NewNamedReader returns a NamedReader that reports filename as its multipart name.
func NewNamedReader(r io.Reader, filename string) NamedReader {
	return NamedReader{Reader: r, name: filename}
}

// Filename satisfies the SDK's optional naming interface.
func (r NamedReader) Filename() string { return r.name }

// Compile-time guarantee that NamedReader keeps satisfying the SDK's
// optional naming interface. If a future refactor embeds *os.File
// (which would promote `Name()` and silently regress to basename-only
// upload names via the SDK's `path.Base(Name())` fallback) or moves
// `Filename()` to a pointer receiver, this assertion fails to compile.
var _ interface{ Filename() string } = NamedReader{}

// backoff returns the delay before the given retry attempt.
// Replaced by tests to avoid real sleeps.
var backoff = func(attempt int) time.Duration {
	return time.Duration(attempt) * 5 * time.Second
}

// MultipartUpload opens filePaths fresh on each attempt and calls fn with the
// resulting readers, retrying up to 3 times on 5xx API errors with backoff.
//
// Each file's multipart name is `dirName + "/" + <relPath>`, where relPath is
// the file's path relative to bundleRoot, using forward slashes regardless of
// the host OS. This preserves nested subdirectory structure inside the bundle
// (e.g. `references/foo.md`) so the Managed Agent runtime can resolve the
// same relative paths the bundle author authored against. If a file lives at
// the top of bundleRoot, relPath is its base name, equivalent to the previous
// flat behaviour.
//
// `dirName` is the API-required top-level directory name in the upload body
// and must match the `name` field of the bundle's `SKILL.md` frontmatter.
//
// A file path that equals bundleRoot (relPath `.`), lies outside it
// (relPath starting with `..`, or `filepath.Rel` failing because the inputs
// are on different volumes or mix absolute/relative forms), returns an
// explicit error rather than silently uploading something nonsensical.
//
// File-open errors and non-5xx API errors are returned immediately without
// retrying.
func MultipartUpload[T any](ctx context.Context, filePaths []string, bundleRoot, dirName string, fn func([]io.Reader) (T, error)) (T, error) {
	const maxAttempts = 3
	var zero T
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}

		files, openedFiles, err := openFiles(filePaths, bundleRoot, dirName)
		if err != nil {
			return zero, err
		}

		result, err := fn(files)
		closeAll(openedFiles)
		if err == nil {
			return result, nil
		}

		var apierr *anthropic.Error
		if !errors.As(err, &apierr) || apierr.StatusCode < 500 || attempt == maxAttempts-1 {
			return zero, err
		}
	}
	return zero, nil // unreachable
}

func openFiles(filePaths []string, bundleRoot, dirName string) ([]io.Reader, []*os.File, error) {
	files := make([]io.Reader, 0, len(filePaths))
	opened := make([]*os.File, 0, len(filePaths))
	for _, p := range filePaths {
		rel, err := filepath.Rel(bundleRoot, p)
		if err != nil {
			closeAll(opened)
			return nil, nil, fmt.Errorf("unable to compute path of %q relative to bundle root %q: %w", p, bundleRoot, err)
		}
		// filepath.IsLocal rejects absolute paths, paths containing `..`
		// segments, empty paths, and Windows reserved names — i.e. anything
		// that would escape bundleRoot or otherwise be invalid as a relative
		// path. It returns true for `.` (the file IS bundleRoot), which is
		// still nonsensical as an upload, so we reject that case explicitly.
		if rel == "." || !filepath.IsLocal(rel) {
			closeAll(opened)
			return nil, nil, fmt.Errorf("file %q is not inside bundle root %q (relative path: %q)", p, bundleRoot, rel)
		}
		f, err := os.Open(p)
		if err != nil {
			closeAll(opened)
			return nil, nil, fmt.Errorf("unable to open file %q: %w", p, err)
		}
		opened = append(opened, f)
		// The API uses forward slashes regardless of host OS; normalise here
		// so Windows-built provider binaries do not emit backslash names.
		uploadName := dirName + "/" + filepath.ToSlash(rel)
		files = append(files, NewNamedReader(f, uploadName))
	}
	return files, opened, nil
}

func closeAll(files []*os.File) {
	for _, f := range files {
		_ = f.Close()
	}
}
