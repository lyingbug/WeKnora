package plugin

import (
	"fmt"
	"strings"
)

// Expressions are how a manifest reaches configuration and input:
//
//	url: ${config.base_url}/search
//	headers:
//	  Authorization: Bearer ${config.api_key}
//	body:
//	  query: ${input.query}
//	  count: ${input.max_results}
//
// The grammar is deliberately tiny — a scope, a dot, a key. No function calls,
// no arithmetic, no conditionals. A plugin file is untrusted input authored by
// someone the deployment may not know, so the expression layer is a lookup
// table rather than a language: there is nothing in it to exploit and nothing
// in it to debug.
//
// Two substitution modes fall out of that, and the distinction is what makes
// the format safe:
//
//   - A value that is exactly one expression resolves to the underlying typed
//     value. `count: ${input.max_results}` sends the JSON number 5.
//   - A value with surrounding text resolves to a string.
//     `Bearer ${config.api_key}` sends the concatenated header.
//
// Because typed values are substituted rather than spliced as text, a search
// query containing a quote or a brace cannot break the JSON body or inject
// into it. A text-template format would make that failure the manifest
// author's problem; this makes it impossible.

// Scope holds the values expressions may read. Each key is a namespace, such
// as "config" or "input"; a domain decides which namespaces it offers.
type Scope map[string]map[string]any

// NewScope returns an empty scope.
func NewScope() Scope { return Scope{} }

// With adds a namespace and returns the scope, for chaining at a call site.
func (s Scope) With(namespace string, values map[string]any) Scope {
	s[namespace] = values
	return s
}

// lookup resolves one dotted reference.
func (s Scope) lookup(ref string) (any, error) {
	namespace, key, found := strings.Cut(ref, ".")
	if !found {
		return nil, fmt.Errorf("${%s}: expected a namespace and a key, such as ${config.api_key}", ref)
	}
	values, ok := s[namespace]
	if !ok {
		return nil, fmt.Errorf("${%s}: unknown namespace %q (available: %s)",
			ref, namespace, s.namespaces())
	}
	value, ok := values[key]
	if !ok {
		// A missing value is not an error. An optional configuration field or
		// an absent input resolves to empty, and the request builder drops
		// empty query parameters and body fields, so a manifest does not need
		// a conditional to express "send this only when it is set".
		return nil, nil
	}
	return value, nil
}

func (s Scope) namespaces() string {
	names := make([]string, 0, len(s))
	for name := range s {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// Resolve substitutes expressions in a template string.
//
// The second result reports whether anything was produced: a lone expression
// resolving to a missing value yields nothing, which lets a caller omit the
// field entirely rather than send an empty one.
func (s Scope) Resolve(template string) (any, bool, error) {
	if !strings.Contains(template, "${") {
		return template, template != "", nil
	}

	// A value that is exactly one expression keeps its type.
	if ref, ok := soleExpression(template); ok {
		value, err := s.lookup(ref)
		if err != nil {
			return nil, false, err
		}
		return value, value != nil && value != "", nil
	}

	// Otherwise the result is text.
	var out strings.Builder
	rest := template
	for {
		before, after, found := strings.Cut(rest, "${")
		out.WriteString(before)
		if !found {
			break
		}
		ref, remainder, closed := strings.Cut(after, "}")
		if !closed {
			return nil, false, fmt.Errorf("unterminated expression in %q: missing '}'", template)
		}
		value, err := s.lookup(strings.TrimSpace(ref))
		if err != nil {
			return nil, false, err
		}
		if value != nil {
			text, err := toString(value)
			if err != nil {
				return nil, false, fmt.Errorf("${%s}: %w", ref, err)
			}
			out.WriteString(text)
		}
		rest = remainder
	}
	result := out.String()
	return result, result != "", nil
}

// ResolveString substitutes expressions and returns the text form.
func (s Scope) ResolveString(template string) (string, error) {
	value, present, err := s.Resolve(template)
	if err != nil {
		return "", err
	}
	if !present || value == nil {
		return "", nil
	}
	return toString(value)
}

// ResolveTree substitutes expressions throughout a structured value, which is
// how a declarative JSON body is built.
//
// A map entry whose value resolves to nothing is dropped rather than sent as
// null, so an optional parameter is simply absent from the request.
func (s Scope) ResolveTree(node any) (any, error) {
	switch typed := node.(type) {
	case string:
		value, present, err := s.Resolve(typed)
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, nil
		}
		return value, nil

	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			resolved, err := s.ResolveTree(child)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			if resolved == nil {
				continue
			}
			out[key] = resolved
		}
		return out, nil

	case map[any]any:
		// YAML may decode a mapping with non-string keys; normalize it.
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			name, err := toString(key)
			if err != nil {
				return nil, fmt.Errorf("map key %v: %w", key, err)
			}
			resolved, err := s.ResolveTree(child)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			if resolved == nil {
				continue
			}
			out[name] = resolved
		}
		return out, nil

	case []any:
		out := make([]any, 0, len(typed))
		for i, child := range typed {
			resolved, err := s.ResolveTree(child)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out = append(out, resolved)
		}
		return out, nil

	default:
		// Numbers, booleans, and nil pass through unchanged.
		return node, nil
	}
}

// soleExpression reports the reference when a template is exactly one
// expression and nothing else.
func soleExpression(template string) (string, bool) {
	trimmed := strings.TrimSpace(template)
	if !strings.HasPrefix(trimmed, "${") || !strings.HasSuffix(trimmed, "}") {
		return "", false
	}
	inner := trimmed[2 : len(trimmed)-1]
	if strings.Contains(inner, "${") || strings.Contains(inner, "}") {
		return "", false
	}
	return strings.TrimSpace(inner), true
}

// validateExpressions checks every expression in a manifest at load time,
// so a typo is reported against the file rather than surfacing as a broken
// request later.
func (m *Manifest) validateExpressions() error {
	if m.Runtime.Type != RuntimeHTTP {
		return nil
	}

	known := map[string]struct{}{}
	for _, field := range m.Config {
		known[field.ID] = struct{}{}
	}

	check := func(where, template string) error {
		for _, ref := range references(template) {
			namespace, key, found := strings.Cut(ref, ".")
			if !found {
				return fmt.Errorf("%s: ${%s} should name a namespace and a key, such as ${config.api_key}", where, ref)
			}
			switch namespace {
			case "config":
				if _, ok := known[key]; !ok {
					return fmt.Errorf("%s: ${%s} refers to a config field that is not declared", where, ref)
				}
			case "input":
				// The set of inputs belongs to the domain, which validates it
				// when the plugin is bound. The kernel only checks the shape.
			default:
				return fmt.Errorf("%s: ${%s} uses unknown namespace %q (expected config or input)",
					where, ref, namespace)
			}
		}
		return nil
	}

	request := m.Runtime.Request
	if err := check("runtime.request.url", request.URL); err != nil {
		return err
	}
	for name, value := range request.Headers {
		if err := check("runtime.request.headers."+name, value); err != nil {
			return err
		}
	}
	for name, value := range request.Query {
		if err := check("runtime.request.query."+name, value); err != nil {
			return err
		}
	}
	return checkTree("runtime.request.body", request.Body, check)
}

func checkTree(where string, node any, check func(string, string) error) error {
	switch typed := node.(type) {
	case string:
		return check(where, typed)
	case map[string]any:
		for key, child := range typed {
			if err := checkTree(where+"."+key, child, check); err != nil {
				return err
			}
		}
	case map[any]any:
		for key, child := range typed {
			name, _ := toString(key)
			if err := checkTree(where+"."+name, child, check); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range typed {
			if err := checkTree(fmt.Sprintf("%s[%d]", where, i), child, check); err != nil {
				return err
			}
		}
	}
	return nil
}

// references reports every expression reference in a template.
func references(template string) []string {
	var out []string
	rest := template
	for {
		_, after, found := strings.Cut(rest, "${")
		if !found {
			return out
		}
		ref, remainder, closed := strings.Cut(after, "}")
		if !closed {
			return out
		}
		out = append(out, strings.TrimSpace(ref))
		rest = remainder
	}
}
