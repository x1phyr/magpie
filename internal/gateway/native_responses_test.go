package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// nativeRequest is a Codex Responses Lite request with everything a native
// Responses backend reads: the additional_tools item (the tool search, the
// functions namespace, a code-mode custom tool), an earlier turn's
// encrypted reasoning, a function call and its output by call_id, and the
// prompt cache key.
var nativeRequest = strings.Replace(liteRequest, `"stream":true,`,
	`"stream":true,"prompt_cache_key":"pck-1","reasoning":{"effort":"high","summary":"auto"},"include":["reasoning.encrypted_content"],`, 1)

// nativeReply is a native Responses stream: reasoning, a function call, a
// tool search call, text, then completed with cached tokens.
var nativeReply = sse(
	`event: response.created`+"\n"+`data: {"type":"response.created","response":{"id":"r1","model":"m1"}}`,
	`event: response.output_item.added`+"\n"+`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"enc"}}`,
	`event: response.reasoning_summary_text.delta`+"\n"+`data: {"type":"response.reasoning_summary_text.delta","item_id":"rs_1","output_index":0,"summary_index":0,"delta":"think"}`,
	`event: response.output_item.done`+"\n"+`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"think"}],"encrypted_content":"enc"}}`,
	`event: response.output_item.done`+"\n"+`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"shell","arguments":"{\"cmd\":\"ls\"}"}}`,
	`event: response.output_text.delta`+"\n"+`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":2,"content_index":0,"delta":"hello"}`,
	`event: response.completed`+"\n"+`data: {"type":"response.completed","response":{"id":"r1","output":[],"usage":{"input_tokens":50,"input_tokens_details":{"cached_tokens":40},"output_tokens":2}}}`)

// setNative sets the fake provider's model native Responses compatible
// (#1571), as the chip's right-click or native-responses.<model>=on does.
func setNative(t *testing.T, model string, on bool) {
	t.Helper()
	if err := provider.SetModelNative("fake/"+model, &on, nil); err != nil {
		t.Fatal(err)
	}
}

func nativePost(t *testing.T, path, body string, lite bool) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	if lite {
		req.Header.Set("X-OpenAI-Internal-Codex-Responses-Lite", "true")
	}
	New().Handler().ServeHTTP(rec, req)
	return rec
}

// eventTypes are the types of an SSE body's events, in order.
func eventTypes(body string) []string {
	var ts []string
	for _, l := range strings.Split(body, "\n") {
		if d, ok := strings.CutPrefix(l, "data: "); ok {
			var e struct {
				Type string `json:"type"`
			}
			if json.Unmarshal([]byte(d), &e) == nil && e.Type != "" {
				ts = append(ts, e.Type)
			}
		}
	}
	return ts
}

// A model set native Responses compatible (#1571) gets Codex's request
// structure as Codex sent it: additional_tools and the tool search not
// lifted nor made a function, the code-mode tool, reasoning, call ids and
// the cache key kept. Its stream comes back to Codex event for event, in
// order, reasoning, call ids and cached tokens in it. Off, the default,
// and for its provider's other models, the request is fitted as before
// (#350).
func TestNativeResponsesPassthrough(t *testing.T) {
	f := &fake{t: t, reply: nativeReply}
	setup(t, provider.Responses, f)

	rec := nativePost(t, "/v1/responses", nativeRequest, true)
	if rec.Code != 200 {
		t.Fatalf("off: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(string(f.got), `"additional_tools"`) || strings.Contains(string(f.got), `"type":"tool_search"`) {
		t.Fatalf("off, the request is fitted as before: %s", f.got)
	}
	wasFitted(t, f.got)
	offEvents := eventTypes(rec.Body.String())

	// another model of the provider set: this one is fitted still
	setNative(t, "m2", true)
	if rec := nativePost(t, "/v1/responses", nativeRequest, true); rec.Code != 200 || strings.Contains(string(f.got), `"additional_tools"`) {
		t.Fatalf("a model not set was sent unfitted: %d %s", rec.Code, f.got)
	}

	setNative(t, "m1", true)
	rec = nativePost(t, "/v1/responses", nativeRequest, true)
	if rec.Code != 200 {
		t.Fatalf("on: %d %s", rec.Code, rec.Body)
	}
	var sent, got map[string]any
	json.Unmarshal([]byte(nativeRequest), &sent)
	if err := json.Unmarshal(f.got, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"input", "prompt_cache_key", "reasoning", "include"} {
		if !reflect.DeepEqual(got[k], sent[k]) {
			t.Errorf("%s changed on its way:\n got %v\nwant %v", k, got[k], sent[k])
		}
	}
	if _, ok := got["tools"]; ok && !reflect.DeepEqual(got["tools"], sent["tools"]) {
		t.Errorf("tools changed: %v", got["tools"])
	}
	if f.head.Get("X-Openai-Internal-Codex-Responses-Lite") == "" && f.head.Get("X-OpenAI-Internal-Codex-Responses-Lite") == "" {
		t.Logf("lite header not forwarded (as before, only codex=as-sent forwards Codex's headers)")
	}
	body := rec.Body.String()
	want := []string{"response.created", "response.output_item.added", "response.reasoning_summary_text.delta", "response.output_item.done", "response.output_item.done", "response.output_text.delta", "response.completed"}
	if ev := eventTypes(body); !reflect.DeepEqual(ev, want) {
		t.Errorf("events %v, want %v", ev, want)
	}
	if !reflect.DeepEqual(eventTypes(body), offEvents) {
		t.Errorf("the reply changed with the setting: on %v off %v", eventTypes(body), offEvents)
	}
	for _, w := range []string{`"call_abc"`, `"encrypted_content":"enc"`, `"cached_tokens":40`, `"delta":"hello"`, `"delta":"think"`} {
		if !strings.Contains(body, w) {
			t.Errorf("reply lacks %s: %s", w, body)
		}
	}
}

// wasFitted checks a request went as magpie fits it for a relay that isn't
// OpenAI's (#350): Responses Lite's tools lifted into tools, the tool
// search a function.
func wasFitted(t *testing.T, got []byte) {
	t.Helper()
	var req struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(got, &req); err != nil {
		t.Fatal(err)
	}
	search, lifted := false, false
	for _, tl := range req.Tools {
		if tl["type"] == "function" && tl["name"] == toolSearch {
			search = true
		}
		if tl["type"] == "namespace" || tl["name"] == "shell" {
			lifted = true
		}
	}
	if !search || !lifted {
		t.Errorf("not fitted (search as function %v, Lite's tools lifted %v): %s", search, lifted, got)
	}
}

// wasNative checks a request went as Codex sent it: additional_tools and the
// tool search left where they were.
func wasNative(t *testing.T, got []byte) {
	t.Helper()
	if !strings.Contains(string(got), `"type":"additional_tools"`) || !strings.Contains(string(got), `"type":"tool_search"`) {
		t.Errorf("not as Codex sent it: %s", got)
	}
}

// A model the user renamed on its way out (settings' ModelWires, one
// model's or the provider's pid/* pattern) is set native by magpie's id
// for it: the request for it goes unfitted under its wire name, and a
// setting under the wire name does nothing.
func TestNativeResponsesRenamedUpstream(t *testing.T) {
	for _, wires := range []map[string]string{{"fake/m1": "wire-m1"}, {"fake/*": "up-*"}} {
		f := &fake{t: t, reply: nativeReply}
		setup(t, provider.Responses, f)
		st := settings.Load()
		st.ModelWires = wires
		st.ModelNative = map[string]string{"fake/wire-m1": "*", "fake/up-m1": "*"}
		if err := settings.Save(st); err != nil {
			t.Fatal(err)
		}
		if rec := nativePost(t, "/v1/responses", nativeRequest, true); rec.Code != 200 {
			t.Fatalf("%v: %d %s", wires, rec.Code, rec.Body)
		}
		if strings.Contains(string(f.got), `"model":"m1"`) {
			t.Fatalf("%v: not renamed: %s", wires, f.got)
		}
		wasFitted(t, f.got)
		setNative(t, "m1", true)
		if rec := nativePost(t, "/v1/responses", nativeRequest, true); rec.Code != 200 {
			t.Fatalf("%v: %d %s", wires, rec.Code, rec.Body)
		}
		wasNative(t, f.got)
	}
}

// A try of a routing group's member is fitted as for any relay, the
// member's model set native or not: a group's entry is offered no native
// capabilities (provider.CodexTiers).
func TestNativeResponsesNotThroughGroups(t *testing.T) {
	f := &fake{t: t, reply: nativeReply}
	setup(t, provider.Responses, f)
	setNative(t, "m1", true)
	p, err := provider.Find("fake")
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []bool{false, true} {
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(nativeRequest))
		if group {
			req = req.WithContext(groupTry(req.Context()))
		}
		rec := httptest.NewRecorder()
		New().passthrough(rec, req, *p, provider.Responses, "m1", []byte(nativeRequest), &Usage{})
		if group {
			wasFitted(t, f.got)
		} else {
			wasNative(t, f.got)
		}
	}
}

// A native Responses provider that turns a Lite request away, or fails in
// its stream, has Codex shown its error: the request is not sent again
// without Lite, and no completion is made up.
func TestNativeResponsesErrors(t *testing.T) {
	f := &fake{t: t}
	setup(t, provider.Responses, f)
	setNative(t, "m1", true)

	f.refuse = func([]byte) (int, string) {
		return http.StatusBadRequest, `{"error":{"message":"unknown item type additional_tools","type":"invalid_request_error"}}`
	}
	f.calls = 0
	rec := nativePost(t, "/v1/responses", nativeRequest, true)
	if rec.Code < 400 || !strings.Contains(rec.Body.String(), "additional_tools") {
		t.Fatalf("a refused Lite request: %d %s", rec.Code, rec.Body)
	}
	if f.calls != 1 || !strings.Contains(string(f.got), `"type":"additional_tools"`) {
		t.Fatalf("sent %d times, last: %s", f.calls, f.got)
	}

	f.refuse = nil
	f.reply = sse(`data: {"type":"response.created","response":{"id":"r1","model":"m1"}}`,
		`data: {"type":"response.failed","response":{"id":"r1","status":"failed","error":{"code":"server_error","message":"lite broke"}}}`)
	rec = nativePost(t, "/v1/responses", nativeRequest, true)
	if !strings.Contains(rec.Body.String(), "lite broke") || strings.Contains(rec.Body.String(), "response.completed") {
		t.Fatalf("an error event: %d %s", rec.Code, rec.Body)
	}

	// cut off before it completes
	f.reply = sse(`data: {"type":"response.created","response":{"id":"r1","model":"m1"}}`,
		`data: {"type":"response.output_text.delta","item_id":"m","output_index":0,"content_index":0,"delta":"hal"}`)
	rec = nativePost(t, "/v1/responses", nativeRequest, true)
	if strings.Contains(rec.Body.String(), `"response.completed"`) {
		t.Fatalf("an interrupted stream completed: %s", rec.Body)
	}
}

// The setting is for a Responses upstream alone: Chat Completions and an
// Anthropic client (Claude Code) are sent the same whether it is on or off.
func TestNativeResponsesOtherProtocols(t *testing.T) {
	for _, c := range []struct {
		name  string
		proto provider.Protocol
		path  string
		body  string
		reply string
	}{
		{"responses to chat", provider.Chat, "/v1/responses", nativeRequest, sse(`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"hi"}}]}`, `data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, `data: [DONE]`)},
		{"chat to chat", provider.Chat, "/v1/chat/completions", `{"model":"fake/m1","stream":true,"messages":[{"role":"user","content":"hi"}]}`, sse(`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"hi"}}]}`, `data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, `data: [DONE]`)},
		{"claude code to responses", provider.Responses, "/v1/messages", `{"model":"fake/m1","max_tokens":100,"stream":true,"tools":[{"name":"Bash","description":"run","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"list"}]}`, nativeReply},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fake{t: t, reply: c.reply}
			setup(t, c.proto, f)
			off := nativePost(t, c.path, c.body, false)
			offGot := string(f.got)
			// as kept, even where the provider couldn't take it now (its
			// Responses URL gone): nothing changes for these
			st := settings.Load()
			st.ModelNative = map[string]string{"fake/m1": "*"}
			if err := settings.Save(st); err != nil {
				t.Fatal(err)
			}
			on := nativePost(t, c.path, c.body, false)
			if on.Code != off.Code || string(f.got) != offGot {
				t.Errorf("sent differently with the setting on:\noff %d %s\non  %d %s", off.Code, offGot, on.Code, f.got)
			}
			if !reflect.DeepEqual(eventTypes(on.Body.String()), eventTypes(off.Body.String())) {
				t.Errorf("answered differently: off %s on %s", off.Body, on.Body)
			}
		})
	}
}
