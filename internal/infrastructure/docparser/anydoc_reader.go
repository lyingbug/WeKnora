package docparser

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tencent/WeKnora/internal/infrastructure/docparser/anydoc"
	"github.com/Tencent/WeKnora/internal/types"
)

// anydocImageDir is the markdown path prefix for extracted images. The image
// resolver matches references by this path and swaps them for storage URLs.
const anydocImageDir = "images/"

// AnydocReader converts office documents to markdown in this process, using
// the anydoc converter instead of the Python docreader service.
//
// It handles the text side of a document only: anydoc extracts embedded images
// as raw bytes but drops them from the markdown (markdown cannot carry bytes),
// so images are appended at the end of the document rather than kept in place.
// Scanned PDFs come out empty, having no text layer — those belong on an OCR
// engine (builtin, mineru, paddleocr_vl).
type AnydocReader struct {
	// extractImages controls the second parse that collects embedded images.
	extractImages bool
}

// NewAnydocReader builds a reader. The "anydoc_extract_images" override turns
// off image extraction, which halves parse time for text-only indexing.
func NewAnydocReader(overrides map[string]string) *AnydocReader {
	return &AnydocReader{extractImages: !isFalsey(overrides["anydoc_extract_images"])}
}

// Read converts the document carried by the request.
func (r *AnydocReader) Read(_ context.Context, req *types.ReadRequest) (*types.ReadResult, error) {
	if req.URL != "" && len(req.FileContent) == 0 {
		return nil, fmt.Errorf("anydoc engine reads uploaded documents, not URLs")
	}

	format, ok := anydoc.FormatForFile(req.FileType, req.FileName)
	if !ok {
		return nil, fmt.Errorf("anydoc engine does not support file type %q", fileTypeOf(req))
	}

	converted, err := anydoc.Convert(req.FileContent, anydoc.Options{
		Format:     format,
		WithAssets: r.extractImages,
	})
	if err != nil {
		return nil, fmt.Errorf("anydoc conversion failed for %q: %w", req.FileName, err)
	}

	markdown, imageRefs := appendAnydocAssets(converted.Markdown, converted.Assets)
	return &types.ReadResult{
		MarkdownContent: markdown,
		ImageRefs:       imageRefs,
		Metadata: map[string]string{
			"parser":         AnydocEngineName,
			"anydoc_version": anydoc.Version(),
			"source_format":  format,
		},
	}, nil
}

// appendAnydocAssets adds one markdown image reference per extracted image, so
// the images reach storage and the multimodal pipeline.
//
// They are appended rather than inlined because the converter reports no
// position for them. What it does report — the author's alt text and the
// heading the image sat under — goes into the link label, so the image still
// carries a description into the chunk it lands in, and the caption model has
// something to work with.
func appendAnydocAssets(markdown string, assets []anydoc.Asset) (string, []types.ImageRef) {
	if len(assets) == 0 {
		return markdown, nil
	}

	refs := make([]types.ImageRef, 0, len(assets))
	var appended strings.Builder
	for _, asset := range assets {
		ref := anydocImageDir + asset.Name
		appended.WriteString(fmt.Sprintf("![%s](%s)\n\n", anydocImageLabel(asset), ref))
		refs = append(refs, types.ImageRef{
			Filename:    asset.Name,
			OriginalRef: ref,
			MimeType:    asset.MediaType,
			ImageData:   asset.Data,
		})
	}

	markdown = strings.TrimRight(markdown, "\n")
	if markdown == "" {
		return strings.TrimRight(appended.String(), "\n"), refs
	}
	return markdown + "\n\n" + strings.TrimRight(appended.String(), "\n"), refs
}

// anydocImageLabel describes an image as well as the document allows: its alt
// text, the section it came from, or failing both, its file name. Brackets are
// stripped because the label sits inside a markdown link label.
func anydocImageLabel(asset anydoc.Asset) string {
	parts := make([]string, 0, 2)
	if alt := sanitizeImageLabel(asset.Alt); alt != "" {
		parts = append(parts, alt)
	}
	if section := sanitizeImageLabel(asset.Section); section != "" {
		parts = append(parts, section)
	}
	if len(parts) == 0 {
		return asset.Name
	}
	return strings.Join(parts, " · ")
}

func sanitizeImageLabel(text string) string {
	text = strings.Map(func(r rune) rune {
		switch r {
		case '[', ']', '\n', '\r':
			return ' '
		}
		return r
	}, text)
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

// fileTypeOf reports the request's file type, falling back to the file name's
// extension the way every reader in this package does.
func fileTypeOf(req *types.ReadRequest) string {
	if ft := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(req.FileType)), "."); ft != "" {
		return ft
	}
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(req.FileName)), ".")
}

func isFalsey(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "no", "off":
		return true
	}
	return false
}
