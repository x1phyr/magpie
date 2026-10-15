package provider

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/yetone/magpie/internal/settings"
)

// A model of a provider the user added by its address, asked on a
// Responses API, can be said to be served as OpenAI's native Responses API
// serves it (#1571: a Sub2API relay with OpenAI's own GPT models behind
// it). Codex's entry for it then takes the prompt and tool capabilities of
// Codex's own entry for the model it maps to (codexcat.NativeSlug: the
// slug the user named, else its id, without the provider or a snapshot's
// date), and the gateway sends a Responses request for it as it would to
// OpenAI: Responses Lite's additional_tools and the tool search as Codex
// sent them. It is said of each model alone (settings' ModelNative), off
// by default, and does nothing on a provider without a Responses URL.

// nativeSlug is what a native slug the user names may be: an OpenAI
// model's (gpt-6.1-sol, o4-mini).
var nativeSlug = regexp.MustCompile(`^(gpt-|o\d)[a-z0-9._-]*$`)

// NativeCapable is whether p's models can be set native Responses
// compatible: a provider the user added by its address with a Responses
// URL.
func (p Provider) NativeCapable() bool {
	return p.Preset == "" && p.Account == nil && p.ID != "" && p.Responses != "" && !p.IsRemoteMagpie()
}

// NativeModel is whether p's model is set native Responses compatible, and
// the slug of Codex's own entry the user named for it ("" to map it by its
// id). Never on a provider that isn't NativeCapable.
func (p Provider) NativeModel(model string) (on bool, slug string) {
	return nativeModelIn(heldSettings(), p, model)
}

func nativeModelIn(s settings.Settings, p Provider, model string) (bool, string) {
	if !p.NativeCapable() {
		return false, ""
	}
	v, ok := s.ModelNative[p.ID+"/"+model]
	if !ok || v == "" {
		return false, ""
	}
	// asked on another API alone, it isn't asked as Responses is
	if api, set := p.ModelAPI(model); set && api != Responses {
		return false, ""
	}
	if v == "*" {
		return true, ""
	}
	return true, v
}

// SetModelNative says whether a provider's model, spelt "provider/model",
// is native Responses compatible (on nil: as it is), and which of Codex's
// own entries it is (slug nil: as it is, "" by its id). Only one value is
// kept for a model, so a slug can't wait on a model that is off: a real
// slug named for one (on nil) turns it on, as naming it says it is that
// model; "" never does, and on a model that is off changes nothing.
func SetModelNative(ref string, on *bool, slug *string) error {
	c, err := setModelNative(ref, on, slug)
	return touchedIf(c, err)
}

func setModelNative(ref string, on *bool, slug *string) (bool, error) {
	p, model, err := splitRef(ref)
	if err != nil {
		return false, err
	}
	s := settings.Load()
	key := p.ID + "/" + model
	was, ok := s.ModelNative[key]
	now := was
	if slug != nil {
		v := strings.ToLower(strings.TrimSpace(*slug))
		if v != "" && !nativeSlug.MatchString(v) {
			return false, fmt.Errorf("%q is not an OpenAI model's slug, as gpt-6.1-sol", *slug)
		}
		if v == "" {
			v = "*"
		}
		switch {
		case on != nil && !*on:
			// off wins below
		case ok, on != nil && *on, v != "*":
			now = v
		}
	}
	if on != nil {
		switch {
		case !*on:
			now = ""
		case now == "":
			now = "*"
		}
	}
	// model ids are kept as the provider spells them: one it doesn't
	// serve, or spelt another way, would be a key no request reaches
	if now != "" && now != was && !p.serves(model) {
		for _, m := range append(p.Available(), p.Exposed()...) {
			if strings.EqualFold(m.ID, model) {
				return false, fmt.Errorf("%s has no model %s: did you mean %s? (model ids are case-sensitive)", p.ID, model, m.ID)
			}
		}
		return false, fmt.Errorf("%s has no model %s (magpie provider %s lists them)", p.ID, model, p.ID)
	}
	if now != "" && !p.NativeCapable() {
		return false, errors.New(p.ID + ": only a provider added by its address with a Responses URL can serve a model as OpenAI's native Responses API does")
	}
	if now == was {
		return false, nil
	}
	if now == "" {
		delete(s.ModelNative, key)
	} else {
		if s.ModelNative == nil {
			s.ModelNative = map[string]string{}
		}
		s.ModelNative[key] = now
	}
	return true, settings.Save(s)
}
