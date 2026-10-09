package traffictesting

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pedrobarco/mroki/pkg/shadowadapter/graphql"
)

// ShadowAdapterGraphQL is the adapter type for a GraphQL shadow service
// compared against a REST live service (pkg/shadowadapter/graphql).
const ShadowAdapterGraphQL = "graphql"

// shadowAdapterVersionLength is the number of hex characters of the config
// hash kept as the version. 12 characters (48 bits) is plenty to tell the
// mappings of a single gate apart.
const shadowAdapterVersionLength = 12

// ShadowAdapter is a gate's optional shadow adapter: it makes a shadow service
// that speaks a different protocol comparable with live. The proxy uses it to
// rewrite shadow requests and the API uses it to normalize both bodies before
// diffing.
//
// The zero value means "no adapter": requests are mirrored and compared as is.
type ShadowAdapter struct {
	typ    string
	config string
}

// ParseShadowAdapter validates an adapter type and its configuration document.
// For "graphql", config is the YAML mapping (the same file the standalone
// proxy reads from MROKI_APP_GRAPHQL_CONFIG).
func ParseShadowAdapter(typ, config string) (ShadowAdapter, error) {
	typ = strings.TrimSpace(typ)
	switch typ {
	case ShadowAdapterGraphQL:
		cfg, err := graphql.ParseConfig([]byte(config))
		if err != nil {
			return ShadowAdapter{}, fmt.Errorf("%w: %v", ErrInvalidShadowAdapter, err)
		}
		// New also checks that the routes can be registered together
		// (conflicting patterns are only detected there).
		if _, err := graphql.New(cfg); err != nil {
			return ShadowAdapter{}, fmt.Errorf("%w: %v", ErrInvalidShadowAdapter, err)
		}
	case "":
		return ShadowAdapter{}, fmt.Errorf("%w: type is required", ErrInvalidShadowAdapter)
	default:
		return ShadowAdapter{}, fmt.Errorf("%w: unsupported type %q (supported: %q)", ErrInvalidShadowAdapter, typ, ShadowAdapterGraphQL)
	}
	return ShadowAdapter{typ: typ, config: config}, nil
}

// NoShadowAdapter returns the zero ShadowAdapter (requests are compared as is).
func NoShadowAdapter() ShadowAdapter {
	return ShadowAdapter{}
}

// IsSet reports whether the gate has a shadow adapter.
func (a ShadowAdapter) IsSet() bool {
	return a.typ != ""
}

// Type returns the adapter type (e.g. "graphql"), or "" when not set.
func (a ShadowAdapter) Type() string {
	return a.typ
}

// Config returns the adapter configuration document, exactly as stored.
func (a ShadowAdapter) Config() string {
	return a.config
}

// Version identifies the mapping a comparison was made with: the first hex
// characters of the SHA-256 of the type and the configuration document. It is
// derived from the content, so the same mapping always has the same version
// and any edit (even a comment) produces a new one. Empty when not set.
func (a ShadowAdapter) Version() string {
	if !a.IsSet() {
		return ""
	}
	sum := sha256.Sum256([]byte(a.typ + "\n" + a.config))
	return hex.EncodeToString(sum[:])[:shadowAdapterVersionLength]
}

// Snapshot returns the adapter reference recorded on each diff computed with
// it. The zero snapshot means no adapter was used.
func (a ShadowAdapter) Snapshot() ShadowAdapterSnapshot {
	if !a.IsSet() {
		return ShadowAdapterSnapshot{}
	}
	return ShadowAdapterSnapshot{Type: a.typ, Version: a.Version()}
}

// ShadowAdapterSnapshot records which adapter (and which version of its
// mapping) a diff was computed with. When set, only bodies were compared.
type ShadowAdapterSnapshot struct {
	Type    string
	Version string
}

// IsZero reports whether no adapter was used.
func (s ShadowAdapterSnapshot) IsZero() bool {
	return s.Type == ""
}
