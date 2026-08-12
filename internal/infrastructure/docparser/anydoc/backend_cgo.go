//go:build anydoc && cgo

package anydoc

import (
	"fmt"
	"mime"
	"strings"

	upstream "github.com/firecrawl/anydoc/go"
)

// This build links the anydoc Rust archive through the upstream cgo bindings
// (vendored under third_party/anydoc-go until they are published as a module).
// Conversion runs in-process: no subprocess, no HTTP hop, no Python.

func backendAvailable() bool { return true }

func backendUnavailableReason() string { return "" }

func backendVersion() string { return upstream.Version }

func backendConvert(data []byte, opts Options) (*Result, error) {
	format, err := upstreamFormat(opts.Format)
	if err != nil {
		return nil, err
	}

	markdown, err := upstream.ToMarkdownBytes(data, format)
	if err != nil {
		return nil, fmt.Errorf("anydoc: markdown conversion failed: %w", err)
	}

	result := &Result{Markdown: markdown}
	if !opts.WithAssets {
		return result, nil
	}

	// Markdown drops embedded images (Markdown cannot carry bytes), so the
	// images come from a second pass over the document model. A failure here
	// is not fatal: text without images is still a usable document, and the
	// caller has no better recovery than the text it already has.
	document, err := upstream.ToDocument(data, format)
	if err != nil {
		return result, nil
	}
	result.Assets = collectAssets(document)
	return result, nil
}

// upstreamFormat maps our format name onto the binding's constant. An empty
// name means "detect from content".
func upstreamFormat(name string) (*upstream.Format, error) {
	if name == "" {
		return nil, nil
	}
	format, ok := upstream.FormatFromExtension(name)
	if !ok {
		return nil, fmt.Errorf("anydoc: unsupported format %q", name)
	}
	return &format, nil
}

// collectAssets names each embedded image after its position in the document,
// because containers store assets without a usable file name of their own.
func collectAssets(document *upstream.Document) []Asset {
	if document == nil || len(document.Assets) == 0 {
		return nil
	}
	assets := make([]Asset, 0, len(document.Assets))
	for i, asset := range document.Assets {
		if len(asset.Data) == 0 {
			continue
		}
		assets = append(assets, Asset{
			Name:      fmt.Sprintf("image-%d%s", i+1, extensionFor(asset.MediaType)),
			MediaType: asset.MediaType,
			Data:      asset.Data,
		})
	}
	return assets
}

func extensionFor(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "image/svg+xml":
		return ".svg"
	}
	// Anything else (EMF/WMF drawings, unknown object payloads) keeps
	// whatever extension the media type registry knows, and .bin otherwise,
	// so downstream storage never writes an extension-less blob.
	if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ".bin"
}
