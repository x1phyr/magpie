package codexcat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
)

// nativeCache is Codex's own catalog with one OpenAI model in it, as a
// ChatGPT sign-in has it.
const nativeCache = `{"etag":"W/\"a\"","models":[{"slug":"gpt-6.1-sol","base_instructions":"sol's own prompt","tool_mode":"code_mode_only","use_responses_lite":true,"experimental_supported_tools":["send_user_message_async","clock"],"supports_search_tool":true,"supports_parallel_tool_calls":true,"shell_type":"shell_command","apply_patch_tool_type":"function","supported_reasoning_levels":[{"effort":"xhigh","description":"x"}],"input_modalities":["text","image"],"supports_reasoning_summaries":true,"multi_agent_version":"v2","context_window":999999,"service_tiers":[{"id":"priority","name":"Fast","description":"f"}]}]}`

func entries(t *testing.T, ms []catalog.Model) map[string]map[string]any {
	t.Helper()
	var got struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(Catalog(ms), &got); err != nil {
		t.Fatal(err)
	}
	by := map[string]map[string]any{}
	for _, m := range got.Models {
		by[m["slug"].(string)] = m
	}
	return by
}

func generic(t *testing.T, slug string, e map[string]any) {
	t.Helper()
	if e == nil {
		t.Fatalf("%s: no entry", slug)
	}
	if _, ok := e["tool_mode"]; ok || e["use_responses_lite"] == true {
		t.Errorf("%s: generic entry wanted, got tool_mode %v lite %v", slug, e["tool_mode"], e["use_responses_lite"])
	}
	if ts, _ := e["experimental_supported_tools"].([]any); len(ts) != 0 {
		t.Errorf("%s: tools %v", slug, ts)
	}
	if e["base_instructions"] != Prompt || e["shell_type"] != "unified_exec" || e["apply_patch_tool_type"] != "freeform" {
		t.Errorf("%s: not magpie's prompt and tools: shell %v patch %v", slug, e["shell_type"], e["apply_patch_tool_type"])
	}
}

// A model of a provider set native Responses compatible (#1571) takes from
// Codex's own entry for the model it maps to — by its id, without the
// provider, without a snapshot's date, or by the alias the user named — the
// prompt verbatim and the tool capabilities, and keeps magpie's own id,
// window, efforts, input kinds and summaries; never the multi-agent
// version. Off (the default), unknown, non-OpenAI or ambiguous: the generic
// entry.
func TestNativeResponsesEntries(t *testing.T) {
	v1Home(t, nativeCache)
	ctx := 400000
	by := entries(t, []catalog.Model{
		{ID: "s2a/gpt-6.1-sol", Name: "a", OwnTier: true, NativeResponses: true, Context: ctx, Efforts: []string{"low", "high"}},
		{ID: "s2a/gpt-6.1-sol-2026-09-01", Name: "b", OwnTier: true, NativeResponses: true},
		{ID: "tx/tx-gpt-6.1-sol-standard", Name: "c", OwnTier: true, NativeResponses: true, NativeAlias: "gpt-6.1-sol"},
		{ID: "tx/tx-gpt-6.1-sol-standard-noalias", Name: "c2", OwnTier: true, NativeResponses: true},
		{ID: "s2a/gpt-9-unknown", Name: "d", OwnTier: true, NativeResponses: true},
		{ID: "s2a/glm-5", Name: "e", OwnTier: true, NativeResponses: true},
		{ID: "off/gpt-6.1-sol", Name: "f", OwnTier: true},
	})
	for _, s := range []string{"s2a/gpt-6.1-sol", "s2a/gpt-6.1-sol-2026-09-01", "tx/tx-gpt-6.1-sol-standard"} {
		e := by[s]
		if e["base_instructions"] != "sol's own prompt" {
			t.Errorf("%s: prompt %.30q, want Codex's own verbatim", s, e["base_instructions"])
		}
		if e["tool_mode"] != "code_mode_only" || e["use_responses_lite"] != true || e["shell_type"] != "shell_command" || e["apply_patch_tool_type"] != "function" || e["supports_search_tool"] != true || e["supports_parallel_tool_calls"] != true {
			t.Errorf("%s: capabilities not taken: %v", s, e)
		}
		if ts, _ := e["experimental_supported_tools"].([]any); len(ts) != 2 || ts[0] != "send_user_message_async" || ts[1] != "clock" {
			t.Errorf("%s: tools %v", s, e["experimental_supported_tools"])
		}
		if _, ok := e["multi_agent_version"]; ok {
			t.Errorf("%s: multi_agent_version taken from Codex's own entry", s)
		}
		if m, _ := e["input_modalities"].([]any); len(m) != 1 {
			t.Errorf("%s: input kinds %v, want magpie's", s, m)
		}
	}
	a := by["s2a/gpt-6.1-sol"]
	if a["context_window"] == float64(999999) || a["max_context_window"] != float64(ctx) || len(a["supported_reasoning_levels"].([]any)) != 2 || a["display_name"] != "a" {
		t.Errorf("magpie's window, efforts and name not kept: %v %v %v", a["context_window"], a["supported_reasoning_levels"], a["display_name"])
	}
	for _, s := range []string{"tx/tx-gpt-6.1-sol-standard-noalias", "s2a/gpt-9-unknown", "s2a/glm-5", "off/gpt-6.1-sol"} {
		generic(t, s, by[s])
	}
}

// Two entries of Codex's own that an id matches by case alone: neither is
// picked.
func TestNativeSlugAmbiguous(t *testing.T) {
	own := map[string]map[string]any{"GPT-6.1-sol": {}, "gpt-6.1-SOL": {}, "gpt-6.1-sol-x": {}}
	if s, ok := NativeSlug(own, catalog.Model{ID: "r/Gpt-6.1-Sol"}); ok {
		t.Errorf("ambiguous id mapped to %q", s)
	}
	if s, ok := NativeSlug(own, catalog.Model{ID: "r/gpt-6.1-sol-x"}); !ok || s != "gpt-6.1-sol-x" {
		t.Errorf("exact match: %q %v", s, ok)
	}
	if _, ok := NativeSlug(own, catalog.Model{ID: "r/claude-x", NativeAlias: "claude-x"}); ok {
		t.Error("a non-OpenAI model mapped")
	}
}

// Without Codex's own catalog on this machine, nothing maps: the generic
// entry, magpie's prompt.
func TestNativeResponsesNoCatalog(t *testing.T) {
	v1Home(t, ``)
	by := entries(t, []catalog.Model{{ID: "s2a/gpt-6.1-sol", Name: "a", OwnTier: true, NativeResponses: true}})
	generic(t, "s2a/gpt-6.1-sol", by["s2a/gpt-6.1-sol"])
}

// A model Codex's own entry has an empty prompt for keeps magpie's.
func TestNativeResponsesEmptyPrompt(t *testing.T) {
	v1Home(t, strings.Replace(nativeCache, `"sol's own prompt"`, `""`, 1))
	e := entries(t, []catalog.Model{{ID: "s2a/gpt-6.1-sol", Name: "a", OwnTier: true, NativeResponses: true}})["s2a/gpt-6.1-sol"]
	if e["base_instructions"] != Prompt || e["tool_mode"] != "code_mode_only" {
		t.Errorf("prompt %.30q tool_mode %v", e["base_instructions"], e["tool_mode"])
	}
}

// A ChatGPT account's own model is Codex's own entry as before, whatever
// any relay is set to.
func TestNativeResponsesLeavesCodexOwn(t *testing.T) {
	v1Home(t, nativeCache)
	e := entries(t, []catalog.Model{{ID: "codex/gpt-6.1-sol", Name: "a"}})["codex/gpt-6.1-sol"]
	if e["base_instructions"] != "sol's own prompt" || e["multi_agent_version"] != "v2" || e["use_responses_lite"] != true {
		t.Errorf("Codex's own entry changed: %v", e)
	}
}
