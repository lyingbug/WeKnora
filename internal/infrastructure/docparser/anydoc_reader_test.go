package docparser

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/infrastructure/docparser/anydoc"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestAnydocReaderRejectsUnsupportedFileType(t *testing.T) {
	_, err := NewAnydocReader(nil).Read(context.Background(), &types.ReadRequest{
		FileName:    "photo.png",
		FileType:    "png",
		FileContent: []byte("not really a png"),
	})
	if err == nil {
		t.Fatal("Read succeeded for an unsupported file type, want an error")
	}
	if !strings.Contains(err.Error(), "png") {
		t.Errorf("error does not name the file type: %v", err)
	}
}

func TestAnydocReaderRejectsURLs(t *testing.T) {
	_, err := NewAnydocReader(nil).Read(context.Background(), &types.ReadRequest{
		URL:      "https://example.com/report.docx",
		FileType: "docx",
	})
	if err == nil {
		t.Fatal("Read succeeded for a URL request, want an error")
	}
}

func TestNewAnydocReaderHonoursImageExtractionOverride(t *testing.T) {
	if !NewAnydocReader(nil).extractImages {
		t.Error("image extraction is off by default, want on")
	}
	if NewAnydocReader(map[string]string{"anydoc_extract_images": "false"}).extractImages {
		t.Error("image extraction stayed on after the override turned it off")
	}
}

// Embedded images arrive as bytes with no position in the markdown, so the
// reader appends a reference per image. The image resolver later matches those
// references against ImageRefs to swap in storage URLs, so the two must agree.
func TestAppendAnydocAssetsLinksEveryImage(t *testing.T) {
	markdown, refs := appendAnydocAssets("# Report\n\nBody text.\n", []anydoc.Asset{
		{Name: "image-1.png", MediaType: "image/png", Data: []byte("first")},
		{Name: "image-2.jpg", MediaType: "image/jpeg", Data: []byte("second")},
	})

	if !strings.HasPrefix(markdown, "# Report\n\nBody text.") {
		t.Fatalf("original markdown was not preserved:\n%s", markdown)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d image refs, want 2", len(refs))
	}
	for _, ref := range refs {
		if !strings.Contains(markdown, "]("+ref.OriginalRef+")") {
			t.Errorf("markdown has no reference for %q:\n%s", ref.OriginalRef, markdown)
		}
		if len(ref.ImageData) == 0 {
			t.Errorf("image ref %q carries no bytes", ref.OriginalRef)
		}
	}
	if refs[0].MimeType != "image/png" {
		t.Errorf("first image mime type = %q, want image/png", refs[0].MimeType)
	}
}

// The appended reference is the only description an image gets, so the label
// carries whatever the document knew about it.
func TestAppendAnydocAssetsLabelsImagesWithTheirContext(t *testing.T) {
	cases := []struct {
		name  string
		asset anydoc.Asset
		want  string
	}{
		{
			name:  "alt text and section",
			asset: anydoc.Asset{Name: "image-1.png", Alt: "出货趋势", Section: "季度经营简报"},
			want:  "![出货趋势 · 季度经营简报](images/image-1.png)",
		},
		{
			name:  "section only",
			asset: anydoc.Asset{Name: "image-1.png", Section: "架构图"},
			want:  "![架构图](images/image-1.png)",
		},
		{
			name:  "file name when the document says nothing",
			asset: anydoc.Asset{Name: "image-1.png"},
			want:  "![image-1.png](images/image-1.png)",
		},
		{
			name:  "brackets and newlines cannot break the link label",
			asset: anydoc.Asset{Name: "image-1.png", Alt: "chart [draft]\nsecond line"},
			want:  "![chart draft second line](images/image-1.png)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			markdown, refs := appendAnydocAssets("Body.", []anydoc.Asset{tc.asset})
			if !strings.Contains(markdown, tc.want) {
				t.Errorf("markdown does not contain %q:\n%s", tc.want, markdown)
			}
			if len(refs) != 1 || refs[0].OriginalRef != "images/image-1.png" {
				t.Errorf("unexpected image refs: %+v", refs)
			}
		})
	}
}

func TestAppendAnydocAssetsLeavesMarkdownAloneWithoutImages(t *testing.T) {
	markdown, refs := appendAnydocAssets("# Report\n", nil)
	if markdown != "# Report\n" {
		t.Errorf("markdown = %q, want it unchanged", markdown)
	}
	if refs != nil {
		t.Errorf("got %d image refs, want none", len(refs))
	}
}
