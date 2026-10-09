package traffictesting

import (
	"encoding/json"
	"strings"
)

// FieldConversion records a declared type conversion applied to one field of
// the shadow body before the diff was computed (see the shadow adapter's
// mapping), so readers can see which values were translated and what the
// shadow actually sent.
type FieldConversion struct {
	// Field is the body field path (dot-separated), e.g. "id".
	Field string
	// As is the type the shadow value was converted to, e.g. "number".
	As string
	// Original is the shadow value as received, JSON-encoded. It is
	// "[REDACTED]" when the field is redacted.
	Original json.RawMessage
	// Error says why the value could not be converted; empty on success. A
	// value that fails keeps its original form, so it shows up in the diff.
	Error string
}

// OK reports whether the value was converted.
func (c FieldConversion) OK() bool {
	return c.Error == ""
}

// WithDiffConversions records the field conversions applied before diffing.
func WithDiffConversions(cs []FieldConversion) diffOption {
	return func(d *Diff) {
		d.Conversions = cs
	}
}

// RedactsBodyPath reports whether a body field path (dot-separated, without
// array wildcards) is redacted, either itself, as part of a redacted object,
// or because it contains a redacted field.
func (r *Redactor) RedactsBodyPath(path string) bool {
	segments := strings.Split(path, ".")
	for _, f := range r.bodyFields {
		if segmentPrefix(strings.Split(f, "."), segments) || segmentPrefix(segments, strings.Split(f, ".")) {
			return true
		}
	}
	return false
}

// segmentPrefix reports whether prefix is a prefix of path, segment by segment.
func segmentPrefix(prefix, path []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if prefix[i] != path[i] {
			return false
		}
	}
	return true
}
