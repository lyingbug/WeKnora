package spi

// Draft is the mutable outbound request an encoder edits. The protocol driver
// builds the canonical body; encoders then write the vendor's non-standard
// fields onto it.
//
// Body is a map rather than a typed struct on purpose. Vendor extensions are
// exactly the fields no shared struct can enumerate — enable_thinking,
// thinking.budget_tokens, chat_template_kwargs, extra_content.google — and a
// typed body would force every one of them through an embedded-struct hack
// plus a separate "must send raw" flag, which is how the previous design
// accumulated four parallel request types. A map also makes the outbound
// bytes directly assertable in tests, so a vendor claim can be checked
// against its documentation.
type Draft struct {
	// Protocol is the wire protocol this draft targets. An encoder written for
	// one protocol must not be attached to a descriptor using another.
	Protocol ProtocolID
	// Model is the model identifier as the vendor expects it.
	Model string
	// Stream reports whether this is a streaming request, which several
	// vendors treat as a hard constraint rather than a preference.
	Stream bool
	// Body is the outbound JSON body.
	Body map[string]any
	// Header carries protocol or vendor headers beyond authentication, such as
	// Anthropic's beta opt-ins.
	Header map[string]string
	// Endpoint overrides the URL the consumer would otherwise derive from the
	// base URL. Empty keeps the protocol's standard path.
	Endpoint string
}

// NewDraft returns a draft with initialized maps.
func NewDraft(protocol ProtocolID, model string, stream bool) *Draft {
	return &Draft{
		Protocol: protocol,
		Model:    model,
		Stream:   stream,
		Body:     map[string]any{},
		Header:   map[string]string{},
	}
}

// Set writes a top-level body field.
func (d *Draft) Set(key string, value any) {
	if d.Body == nil {
		d.Body = map[string]any{}
	}
	d.Body[key] = value
}

// Get reads a top-level body field.
func (d *Draft) Get(key string) (any, bool) {
	if d.Body == nil {
		return nil, false
	}
	v, ok := d.Body[key]
	return v, ok
}

// Delete removes a top-level body field. Used by forbidden parameters, whose
// contract is that the field must not reach the wire at all.
func (d *Draft) Delete(key string) {
	delete(d.Body, key)
}

// SetNested writes value at a nested object path, creating intermediate
// objects as needed. It is how `thinking.type`, `chat_template_kwargs
// .enable_thinking`, and `output_config.effort` are expressed without each
// encoder hand-rolling the same map bookkeeping.
//
// An intermediate key holding a non-object is replaced, because the encoder
// declaring the path owns that subtree.
func (d *Draft) SetNested(value any, path ...string) {
	if len(path) == 0 {
		return
	}
	if d.Body == nil {
		d.Body = map[string]any{}
	}
	node := d.Body
	for _, key := range path[:len(path)-1] {
		child, ok := node[key].(map[string]any)
		if !ok {
			child = map[string]any{}
			node[key] = child
		}
		node = child
	}
	node[path[len(path)-1]] = value
}

// GetNested reads the value at a nested object path.
func (d *Draft) GetNested(path ...string) (any, bool) {
	if len(path) == 0 || d.Body == nil {
		return nil, false
	}
	var node any = d.Body
	for _, key := range path {
		obj, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		node, ok = obj[key]
		if !ok {
			return nil, false
		}
	}
	return node, true
}

// SetHeader records an outbound header.
func (d *Draft) SetHeader(key, value string) {
	if d.Header == nil {
		d.Header = map[string]string{}
	}
	d.Header[key] = value
}

// Rename moves a body field to a different key, preserving its value. It
// expresses the common case of a vendor spelling a standard parameter
// differently, such as OpenAI reasoning models requiring max_completion_tokens
// in place of max_tokens.
func (d *Draft) Rename(from, to string) {
	v, ok := d.Get(from)
	if !ok {
		return
	}
	d.Delete(from)
	d.Set(to, v)
}
