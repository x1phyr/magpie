package provider

import (
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/settings"
)

// Native Responses is said of each model alone (#1571), kept in settings'
// ModelNative: off by default; on, with the slug named for it or by its
// id; never on a provider without a Responses URL, a preset's or an
// account's, nor on a model asked on another API alone. CodexTiers hands
// it to codexcat.
func TestNativeResponsesPerModel(t *testing.T) {
	prefsHome(t)
	relay := Provider{ID: "s2a", Name: "S2A", Key: "k", Chat: "https://r/v1", Responses: "https://r/v1", Models: []string{"tx-gpt-6.1-sol-standard", "gpt-6.1-sol", "gpt-chat"}}
	chat := Provider{ID: "chatonly", Name: "C", Key: "k", Chat: "https://c/v1", Models: []string{"gpt-6.1-sol"}}
	for _, p := range []Provider{relay, chat} {
		if err := Save(p); err != nil {
			t.Fatal(err)
		}
	}
	mark := func(id, model string) (bool, string) {
		t.Helper()
		p, err := Find(id)
		if err != nil {
			t.Fatal(err)
		}
		var m catalog.Model
		CodexTiers(&m, Entry{ID: id + "/" + model, Model: "wire-" + model, Provider: *p}, func(string) []Member { return nil })
		return m.NativeResponses, m.NativeAlias
	}
	if on, _ := mark("s2a", "gpt-6.1-sol"); on {
		t.Fatal("on by default")
	}
	yes, no := true, false
	slug := "GPT-6.1-sol"
	if err := SetModelPrefs("s2a", map[string]ModelPref{
		"tx-gpt-6.1-sol-standard": {Native: &yes, NativeSlug: &slug},
		"gpt-6.1-sol":             {Native: &yes},
		"gpt-chat":                {Native: &yes, API: strp("chat")},
	}); err != nil {
		t.Fatal(err)
	}
	if on, s := mark("s2a", "tx-gpt-6.1-sol-standard"); !on || s != "gpt-6.1-sol" {
		t.Errorf("aliased: %v %q", on, s)
	}
	if on, s := mark("s2a", "gpt-6.1-sol"); !on || s != "" {
		t.Errorf("by its id: %v %q", on, s)
	}
	if on, _ := mark("s2a", "gpt-chat"); on {
		t.Error("a model asked on chat alone is on")
	}
	if err := SetModelNative("chatonly/gpt-6.1-sol", &yes, nil); err == nil {
		t.Error("a provider without a Responses URL took it")
	}
	bad := "claude-x"
	if err := SetModelNative("s2a/gpt-6.1-sol", nil, &bad); err == nil {
		t.Error("a non-OpenAI slug taken")
	}
	if err := SetModelNative("s2a/gpt-6.1-sol", &no, nil); err != nil {
		t.Fatal(err)
	}
	if on, _ := mark("s2a", "gpt-6.1-sol"); on {
		t.Error("off again")
	}
	if _, ok := settings.Load().ModelNative["s2a/gpt-6.1-sol"]; ok {
		t.Error("off left its key")
	}
	if on, _ := mark("s2a", "tx-gpt-6.1-sol-standard"); !on {
		t.Error("another model's setting changed")
	}

	// an empty slug never turns an off model on; a real one does, as
	// naming it says what the model is
	empty, real := "", "gpt-6.1-sol"
	if err := SetModelNative("s2a/gpt-6.1-sol", nil, &empty); err != nil {
		t.Fatal(err)
	}
	if on, _ := mark("s2a", "gpt-6.1-sol"); on {
		t.Error("an empty slug turned it on")
	}
	if err := SetModelNative("s2a/gpt-6.1-sol", nil, &real); err != nil {
		t.Fatal(err)
	}
	if on, s := mark("s2a", "gpt-6.1-sol"); !on || s != "gpt-6.1-sol" {
		t.Errorf("a real slug: %v %q", on, s)
	}
	if err := SetModelNative("s2a/gpt-6.1-sol", nil, &empty); err != nil {
		t.Fatal(err)
	}
	if on, s := mark("s2a", "gpt-6.1-sol"); !on || s != "" {
		t.Errorf("an empty slug on an on model maps it by its id: %v %q", on, s)
	}
	star := "*"
	if err := SetModelNative("s2a/gpt-6.1-sol", nil, &star); err == nil {
		t.Error("\"*\" taken as a slug")
	}

	// a routing group's entry is never marked, its member set or not
	p, _ := Find("s2a")
	var g catalog.Model
	CodexTiers(&g, Entry{ID: "group:x", Model: "tx-gpt-6.1-sol-standard", Group: "x", Provider: *p}, func(string) []Member { return nil })
	if g.NativeResponses {
		t.Error("a group's entry marked native")
	}
}

func strp(s string) *string { return &s }
