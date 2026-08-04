package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func blockPage(count int) []json.RawMessage {
	data := make([]json.RawMessage, count)
	for i := range data {
		data[i] = json.RawMessage(`{"blockType":"paragraph","paragraph":{"text":"body"}}`)
	}
	return data
}

func TestListDocumentBlocksAcceptsResponseWithoutSuccessFlag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, accessTokenResponse{AccessToken: "token", ExpireIn: 7200})
	})
	mux.HandleFunc("/v1.0/doc/suites/documents/doc-1/blocks",
		func(w http.ResponseWriter, _ *http.Request) {
			var response documentBlocksResponse
			response.Result.Data = blockPage(2)
			writeJSON(t, w, response)
		})
	server := httptest.NewServer(mux)
	defer server.Close()

	blocks, err := testHTTPClient(server).listDocumentBlocks(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("listDocumentBlocks() error = %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("listDocumentBlocks() count = %d, want 2", len(blocks))
	}
}

func TestListDocumentBlocksRejectsExplicitFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, accessTokenResponse{AccessToken: "token", ExpireIn: 7200})
	})
	mux.HandleFunc("/v1.0/doc/suites/documents/doc-1/blocks",
		func(w http.ResponseWriter, _ *http.Request) {
			failed := false
			writeJSON(t, w, documentBlocksResponse{Success: &failed})
		})
	server := httptest.NewServer(mux)
	defer server.Close()

	if _, err := testHTTPClient(server).listDocumentBlocks(context.Background(), "doc-1"); err == nil {
		t.Fatal("listDocumentBlocks() error = nil, want failure for success=false")
	}
}

func TestListDocumentBlocksStopsWhenServerIgnoresBlockRange(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, accessTokenResponse{AccessToken: "token", ExpireIn: 7200})
	})
	mux.HandleFunc("/v1.0/doc/suites/documents/doc-1/blocks",
		func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			// Ignore startIndex/endIndex and always return the whole document.
			var response documentBlocksResponse
			response.Result.Data = blockPage(blockPageSize + 5)
			writeJSON(t, w, response)
		})
	server := httptest.NewServer(mux)
	defer server.Close()

	blocks, err := testHTTPClient(server).listDocumentBlocks(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("listDocumentBlocks() error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("block requests = %d, want 1 (paging on would duplicate content)", got)
	}
	if len(blocks) != blockPageSize+5 {
		t.Fatalf("listDocumentBlocks() count = %d, want %d", len(blocks), blockPageSize+5)
	}
}
