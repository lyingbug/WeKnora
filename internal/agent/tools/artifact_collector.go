package tools

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// Artifact collection limits
const (
	MaxArtifacts    = 10
	MaxArtifactSize = 20 * 1024 * 1024 // 20MB per file
	MaxTotalSize    = 50 * 1024 * 1024  // 50MB total
)

// AllowedMimeTypes defines the set of MIME types that can be collected as artifacts.
var AllowedMimeTypes = map[string]bool{
	"application/pdf":  true,
	"text/csv":         true,
	"application/json": true,
	"text/plain":       true,
	"text/html":        true,
	"text/markdown":    true,
	"image/png":        true,
	"image/jpeg":       true,
	"image/svg+xml":    true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
}

// extToMime provides a fallback mapping from file extension to MIME type,
// used when the standard mime package does not recognise the extension.
var extToMime = map[string]string{
	".pdf":  "application/pdf",
	".csv":  "text/csv",
	".json": "application/json",
	".txt":  "text/plain",
	".html": "text/html",
	".htm":  "text/html",
	".md":   "text/markdown",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".svg":  "image/svg+xml",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

// CollectArtifacts walks outputDir and returns an Artifact for every regular
// file that has an allowed MIME type and respects the size limits.
// It returns at most MaxArtifacts items with a cumulative size of MaxTotalSize.
func CollectArtifacts(outputDir string) ([]types.Artifact, error) {
	info, err := os.Stat(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no output directory — no artifacts
		}
		return nil, fmt.Errorf("failed to stat output dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("output path is not a directory: %s", outputDir)
	}

	var artifacts []types.Artifact
	var totalSize int64

	walkErr := filepath.Walk(outputDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip files we cannot stat
		}
		if fi.IsDir() {
			return nil // descend into sub-directories
		}

		// Enforce per-artifact count limit
		if len(artifacts) >= MaxArtifacts {
			return filepath.SkipAll
		}

		// Determine MIME type
		mimeType := mimeFromExt(filepath.Ext(fi.Name()))
		if mimeType == "" || !AllowedMimeTypes[mimeType] {
			return nil // skip unrecognised or disallowed types
		}

		size := fi.Size()

		// Enforce per-file size limit
		if size > MaxArtifactSize {
			return nil
		}

		// Enforce total size limit
		if totalSize+size > MaxTotalSize {
			return nil
		}

		fullPath := path

		// Validate file content matches expected type
		valid, _ := validateFileContent(fullPath, mimeType)
		if !valid {
			// Content does not match extension — skip this file
			return nil
		}

		totalSize += size

		artifacts = append(artifacts, types.Artifact{
			Name:     fi.Name(),
			Path:     path,
			MimeType: mimeType,
			Size:     size,
		})

		return nil
	})

	if walkErr != nil {
		return artifacts, fmt.Errorf("error walking output dir: %w", walkErr)
	}

	return artifacts, nil
}

// FormatArtifactSummary returns a human-readable one-liner such as
// "[Artifacts produced: report.pdf (2.3MB), data.csv (156KB)]".
// If the slice is empty it returns an empty string.
func FormatArtifactSummary(artifacts []types.Artifact) string {
	if len(artifacts) == 0 {
		return ""
	}

	parts := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		parts = append(parts, fmt.Sprintf("%s (%s)", a.Name, humanSize(a.Size)))
	}
	return fmt.Sprintf("[Artifacts produced: %s]", strings.Join(parts, ", "))
}

// validateFileContent checks if the file's actual content matches the expected MIME type
func validateFileContent(path string, expectedMime string) (bool, string) {
	f, err := os.Open(path)
	if err != nil {
		return false, ""
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n == 0 {
		return false, ""
	}
	detectedType := http.DetectContentType(buf[:n])

	return isCompatibleMime(expectedMime, detectedType), detectedType
}

// isCompatibleMime checks if detected content type is compatible with expected MIME type
func isCompatibleMime(expected, detected string) bool {
	// Normalize: remove charset suffix
	detected = strings.Split(detected, ";")[0]
	detected = strings.TrimSpace(detected)

	// Exact match
	if detected == expected {
		return true
	}

	// Text-based formats: DetectContentType often returns "text/plain" for csv, json, md, etc.
	textExpected := map[string]bool{
		"text/csv": true, "text/plain": true, "text/html": true,
		"text/markdown": true, "application/json": true,
	}
	if textExpected[expected] && (strings.HasPrefix(detected, "text/") || detected == "application/json") {
		return true
	}

	// Image types should match at category level
	if strings.HasPrefix(expected, "image/") && strings.HasPrefix(detected, "image/") {
		return true
	}

	// application/octet-stream is a generic fallback - allow for known binary types
	if detected == "application/octet-stream" {
		knownBinary := map[string]bool{
			"application/pdf": true,
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
		}
		return knownBinary[expected]
	}

	return false
}

// mimeFromExt returns a MIME type for the given file extension (including the dot).
func mimeFromExt(ext string) string {
	ext = strings.ToLower(ext)
	// Try the standard library first
	if mt := mime.TypeByExtension(ext); mt != "" {
		// mime.TypeByExtension may return parameters like "text/plain; charset=utf-8"
		mt, _, _ = strings.Cut(mt, ";")
		mt = strings.TrimSpace(mt)
		return mt
	}
	// Fallback to our own map
	return extToMime[ext]
}

// humanSize formats a byte count into a human-readable string (e.g. "2.3MB").
func humanSize(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case b >= mb:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.0fKB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
