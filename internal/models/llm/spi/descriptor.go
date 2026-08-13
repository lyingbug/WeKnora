package spi

import (
	"fmt"
	"regexp"
	"strings"
)

// AuthKind is how a plugin authenticates a request.
type AuthKind string

const (
	// AuthBearer sends `Authorization: Bearer <key>`, the common case.
	AuthBearer AuthKind = "bearer"
	// AuthHeader sends the key in a named header, such as Azure's `api-key`
	// or Anthropic's `x-api-key`.
	AuthHeader AuthKind = "header"
	// AuthSigned delegates to a Signer, for vendors that sign request bytes.
	AuthSigned AuthKind = "signed"
)

// Signer produces authentication headers over the exact outbound bytes. It is
// an interface rather than a declaration because request signing is genuinely
// code, and pretending otherwise would push a signing algorithm into config.
type Signer interface {
	// Sign returns the headers authenticating body for this request.
	Sign(body []byte, creds Credentials) (map[string]string, error)
}

// Credentials carries the secrets a plugin may need. Bearer and header schemes
// use APIKey alone; signing schemes also need the application pair.
type Credentials struct {
	APIKey    string
	AppID     string
	AppSecret string
}

// Auth declares how requests are authenticated.
type Auth struct {
	// Kind selects the scheme; the zero value is AuthBearer.
	Kind AuthKind `json:"kind,omitempty"`
	// Header names the header for AuthHeader.
	Header string `json:"header,omitempty"`
	// Signer implements AuthSigned.
	Signer Signer `json:"-"`
	// Static are headers sent on every request, such as Anthropic's required
	// `anthropic-version`.
	Static map[string]string `json:"static,omitempty"`
}

// EffectiveKind reports the scheme, treating the zero value as AuthBearer.
func (a Auth) EffectiveKind() AuthKind {
	if a.Kind == "" {
		return AuthBearer
	}
	return a.Kind
}

// ModelMatcher selects the models a descriptor applies to, so one vendor can
// publish several descriptors — a reasoning-model descriptor and a general one,
// for instance — without a predicate function that only its author can audit.
// An empty matcher matches every model, which is the vendor's catch-all.
type ModelMatcher struct {
	// Prefixes matches a case-insensitive model-name prefix.
	Prefixes []string `json:"prefixes,omitempty"`
	// Contains matches a case-insensitive substring.
	Contains []string `json:"contains,omitempty"`
	// Pattern matches a regular expression against the lowercased name.
	Pattern string `json:"pattern,omitempty"`

	compiled *regexp.Regexp
}

// IsCatchAll reports whether the matcher accepts every model.
func (m ModelMatcher) IsCatchAll() bool {
	return len(m.Prefixes) == 0 && len(m.Contains) == 0 && m.Pattern == ""
}

// Matches reports whether the matcher accepts a model name.
func (m ModelMatcher) Matches(model string) bool {
	if m.IsCatchAll() {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range m.Prefixes {
		if strings.HasPrefix(name, strings.ToLower(prefix)) {
			return true
		}
	}
	for _, sub := range m.Contains {
		if strings.Contains(name, strings.ToLower(sub)) {
			return true
		}
	}
	if m.compiled != nil {
		return m.compiled.MatchString(name)
	}
	return false
}

// compile prepares the regular expression, reporting a malformed pattern at
// registration time rather than on the first request that uses it.
func (m *ModelMatcher) compile() error {
	if m.Pattern == "" {
		return nil
	}
	re, err := regexp.Compile(m.Pattern)
	if err != nil {
		return fmt.Errorf("compile model pattern %q: %w", m.Pattern, err)
	}
	m.compiled = re
	return nil
}

// ReasoningReplay declares whether prior-turn reasoning must be sent back to
// the vendor. It is a wire requirement, not a preference: DeepSeek returns 400
// when a tool-calling turn's reasoning_content is dropped, and Anthropic
// requires thinking blocks to return unmodified.
type ReasoningReplay string

const (
	// ReplayNever means prior reasoning is ignored and need not be sent.
	ReplayNever ReasoningReplay = "never"
	// ReplayWithTools means reasoning must be replayed on turns that carry
	// tool calls, and may be dropped otherwise.
	ReplayWithTools ReasoningReplay = "with_tools"
	// ReplayAlways means every prior reasoning block must be replayed verbatim.
	ReplayAlways ReasoningReplay = "always"
)

// Descriptor is a vendor plugin: everything that distinguishes one vendor's
// model API from the protocol baseline, declared rather than coded.
//
// A descriptor is registered per (vendor, kind), and a vendor may register
// several for one kind when its models genuinely differ — an OpenAI reasoning
// descriptor beside the general one, for example. Resolution prefers the
// descriptor whose matcher is specific.
type Descriptor struct {
	// Vendor is the provider identity, matching the stored model's provider
	// field, e.g. "deepseek".
	Vendor string
	// Kind is the capability this descriptor serves.
	Kind ModelKind
	// Protocol is the wire protocol the vendor speaks.
	Protocol ProtocolID
	// Models restricts the descriptor to matching model names. An empty
	// matcher makes it the vendor's catch-all.
	Models ModelMatcher
	// DisplayName is the human-readable vendor name for diagnostics; the UI
	// resolves its own label from the provider catalog.
	DisplayName string

	// DefaultBaseURL is used when the model configuration leaves it empty.
	DefaultBaseURL string
	// EndpointPath overrides the protocol's standard path, for vendors serving
	// a compatible protocol from a different route.
	EndpointPath string
	// Auth declares the authentication scheme.
	Auth Auth

	// Params declares every request parameter this plugin exposes, in the
	// order a form should present them.
	Params []Param
	// Constraints are cross-parameter rules applied after resolution and
	// before encoding.
	Constraints []Constraint

	// ReasoningReplay declares whether prior-turn reasoning must be sent back.
	ReasoningReplay ReasoningReplay
	// DocURL points at the vendor documentation this descriptor follows.
	DocURL string
}

// Param reports a declared parameter by id.
func (d Descriptor) Param(id ParamID) (Param, bool) {
	for _, p := range d.Params {
		if p.ID == id {
			return p, true
		}
	}
	return Param{}, false
}

// Supports reports whether the descriptor declares a parameter at all, which
// is what the UI asks before rendering a control and what the debug endpoint
// asks before offering a toggle.
func (d Descriptor) Supports(id ParamID) bool {
	p, ok := d.Param(id)
	return ok && p.EffectiveSupport() != SupportForbidden
}

// EffectiveReplay reports the reasoning-replay rule, treating the zero value
// as ReplayNever.
func (d Descriptor) EffectiveReplay() ReasoningReplay {
	if d.ReasoningReplay == "" {
		return ReplayNever
	}
	return d.ReasoningReplay
}

// validate checks a descriptor's internal consistency at registration time.
// These are the mistakes a plugin author actually makes, and catching them at
// startup beats discovering them through a vendor's 400 in production.
func (d *Descriptor) validate() error {
	if strings.TrimSpace(d.Vendor) == "" {
		return fmt.Errorf("descriptor: vendor is required")
	}
	if d.Kind == "" {
		return fmt.Errorf("descriptor %s: kind is required", d.Vendor)
	}
	if d.Protocol == "" {
		return fmt.Errorf("descriptor %s/%s: protocol is required", d.Vendor, d.Kind)
	}
	if err := d.Models.compile(); err != nil {
		return fmt.Errorf("descriptor %s/%s: %w", d.Vendor, d.Kind, err)
	}
	if d.Auth.EffectiveKind() == AuthHeader && strings.TrimSpace(d.Auth.Header) == "" {
		return fmt.Errorf("descriptor %s/%s: header auth requires a header name", d.Vendor, d.Kind)
	}
	if d.Auth.EffectiveKind() == AuthSigned && d.Auth.Signer == nil {
		return fmt.Errorf("descriptor %s/%s: signed auth requires a signer", d.Vendor, d.Kind)
	}
	seen := make(map[ParamID]struct{}, len(d.Params))
	for _, p := range d.Params {
		if _, dup := seen[p.ID]; dup {
			return fmt.Errorf("descriptor %s/%s: duplicate parameter %s", d.Vendor, d.Kind, p.ID)
		}
		seen[p.ID] = struct{}{}
		if err := validateParam(d, p); err != nil {
			return err
		}
	}
	return nil
}

func validateParam(d *Descriptor, p Param) error {
	where := fmt.Sprintf("descriptor %s/%s parameter %s", d.Vendor, d.Kind, p.ID)
	if p.Kind == "" {
		return fmt.Errorf("%s: kind is required", where)
	}
	if p.Encode == nil {
		return fmt.Errorf("%s: every parameter needs an encoder", where)
	}

	// A forbidden parameter is never sent, so its domain is irrelevant; what it
	// does need is the ability to remove itself, because the protocol driver
	// has already written the canonical field.
	if p.EffectiveSupport() == SupportForbidden {
		if _, ok := p.Encode.(Stripper); !ok {
			return fmt.Errorf("%s: a forbidden parameter needs an encoder that can strip its field, "+
				"otherwise the protocol's own value would still reach the vendor", where)
		}
		return nil
	}

	if p.Kind == KindEnum && len(p.Enum) == 0 {
		return fmt.Errorf("%s: enum parameter needs at least one option", where)
	}
	if p.Kind != KindEnum && len(p.Enum) > 0 {
		return fmt.Errorf("%s: only enum parameters may declare options", where)
	}
	if p.EffectiveSupport() == SupportPinned && p.Pin == nil {
		return fmt.Errorf("%s: pinned parameter needs a pin value", where)
	}
	if p.Pin != nil && !p.AllowsValue(*p.Pin) {
		return fmt.Errorf("%s: pin value %s is outside the declared domain", where, p.Pin)
	}
	if p.Default != nil && !p.AllowsValue(*p.Default) {
		return fmt.Errorf("%s: default value %s is outside the declared domain", where, p.Default)
	}
	if p.Min != nil && p.Max != nil && *p.Min > *p.Max {
		return fmt.Errorf("%s: min %v exceeds max %v", where, *p.Min, *p.Max)
	}
	return nil
}
