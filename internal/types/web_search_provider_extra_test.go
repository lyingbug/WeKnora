package types

import "testing"

func TestRuntimeProviderTypeCatalog(t *testing.T) {
	id := "runtime-catalog-test"
	RegisterWebSearchProviderType(WebSearchProviderTypeInfo{ID: id, Name: "Runtime"})
	t.Cleanup(func() { UnregisterWebSearchProviderType(id) })

	if !IsKnownWebSearchProviderType(id) {
		t.Fatal("expected runtime type")
	}
	if !IsKnownWebSearchProviderType("bing") {
		t.Fatal("builtin bing")
	}
	found := false
	for _, info := range GetWebSearchProviderTypes() {
		if info.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("GetWebSearchProviderTypes should include runtime type")
	}
	UnregisterWebSearchProviderType(id)
	if IsKnownWebSearchProviderType(id) {
		t.Fatal("unregistered type should be gone")
	}
}
