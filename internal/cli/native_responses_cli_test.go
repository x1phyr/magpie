package cli

import (
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// native-responses.<model>=on|off and native-slug.<model>=<slug> are taken
// out of a provider set's pairs as each model's setting (#1571); the rest
// are the provider's as before.
func TestNativePairs(t *testing.T) {
	rest, prefs, err := nativePairs([]string{"name=Relay", "native-responses.tx-gpt-6.1-sol-standard=on", "native-slug.tx-gpt-6.1-sol-standard=gpt-6.1-sol", "native-responses.gpt-6.1-sol=off"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 1 || rest[0] != "name=Relay" {
		t.Errorf("rest %v", rest)
	}
	tx := prefs["tx-gpt-6.1-sol-standard"]
	if tx.Native == nil || !*tx.Native || tx.NativeSlug == nil || *tx.NativeSlug != "gpt-6.1-sol" {
		t.Errorf("tx: %+v", tx)
	}
	if g := prefs["gpt-6.1-sol"]; g.Native == nil || *g.Native {
		t.Errorf("off: %+v", g)
	}
	_, prefs, _ = nativePairs([]string{"native-slug.m="})
	if m := prefs["m"]; m.Native != nil || m.NativeSlug == nil || *m.NativeSlug != "" {
		t.Errorf("an empty slug says nothing of on: %+v", m)
	}
	if !strings.Contains(providerUsage, "native-responses.<model>=on") || !strings.Contains(providerUsage, "native-slug.<model>=") {
		t.Error("the usage doesn't say the native keys")
	}
	if _, _, err := nativePairs([]string{"native-responses.m=maybe"}); err == nil || !strings.Contains(err.Error(), "on|off") {
		t.Errorf("bad value: %v", err)
	}
}

// `magpie provider set` with native-responses.<model> or native-slug.<model>
// for a model the provider doesn't serve, or spelt in another case, is an
// error that says so, and nothing is kept; one it serves is set.
func TestNativeResponsesUnknownModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := provider.Save(provider.Provider{ID: "s2a", Name: "S2A", Key: "k", Responses: "http://127.0.0.1:1/v1", Models: []string{"gpt-6.1-sol"}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ pair, want string }{
		{"native-responses.gpt-9=on", "has no model gpt-9"},
		{"native-slug.gpt-9=gpt-6.1-sol", "has no model gpt-9"},
		{"native-responses.GPT-6.1-Sol=on", "did you mean gpt-6.1-sol"},
	} {
		if err := providerCmd([]string{"provider", "set", "s2a", c.pair}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.pair, err, c.want)
		}
	}
	if len(settings.Load().ModelNative) != 0 {
		t.Errorf("kept: %v", settings.Load().ModelNative)
	}
	if err := providerCmd([]string{"provider", "set", "s2a", "native-responses.gpt-6.1-sol=on"}); err != nil {
		t.Fatal(err)
	}
	if settings.Load().ModelNative["s2a/gpt-6.1-sol"] != "*" {
		t.Errorf("not set: %v", settings.Load().ModelNative)
	}
}
