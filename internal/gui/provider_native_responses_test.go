package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// A model chip's Native Responses (#1571): /api/providers says which
// providers can take it and which models have it, and a Save's modelPrefs
// native and nativeSlug set it for that model alone.
func TestProviderNativeResponsesModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(body string) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/provider/save", strings.NewReader(body)))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	post(`{"id":"relay","name":"Relay","key":"sk-r","responses":"http://127.0.0.1:1/v1","icon":"generic","new":true,"extra":["tx-gpt-6.1-sol-standard","gpt-6.1-sol"],"models":["tx-gpt-6.1-sol-standard","gpt-6.1-sol"]}`)
	p, err := provider.Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if on, _ := p.NativeModel("tx-gpt-6.1-sol-standard"); on {
		t.Fatal("on by default")
	}
	post(`{"id":"relay","from":"relay","name":"Relay","key":"","responses":"http://127.0.0.1:1/v1","icon":"generic","models":["tx-gpt-6.1-sol-standard","gpt-6.1-sol"],"headers":{},"modelPrefs":{"tx-gpt-6.1-sol-standard":{"native":true,"nativeSlug":"gpt-6.1-sol"}}}`)
	if on, slug := p.NativeModel("tx-gpt-6.1-sol-standard"); !on || slug != "gpt-6.1-sol" {
		t.Fatalf("saved: %v %q", on, slug)
	}
	if on, _ := p.NativeModel("gpt-6.1-sol"); on {
		t.Fatal("another model turned on")
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/providers", nil))
	var got struct {
		Providers []struct {
			ID            string `json:"id"`
			NativeCapable bool   `json:"nativeCapable"`
			Models        []struct {
				ID         string `json:"id"`
				Native     bool   `json:"native"`
				NativeSlug string `json:"nativeSlug"`
			} `json:"models"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pr := range got.Providers {
		if pr.ID != "relay" {
			continue
		}
		if !pr.NativeCapable {
			t.Error("relay not nativeCapable")
		}
		for _, m := range pr.Models {
			if m.ID == "tx-gpt-6.1-sol-standard" {
				found = m.Native && m.NativeSlug == "gpt-6.1-sol"
			}
		}
	}
	if !found {
		t.Errorf("the model's setting not listed: %s", w.Body)
	}
}
