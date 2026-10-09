package dto

// GateStats holds computed statistics for a single gate.
type GateStats struct {
	RequestCount24h int64   `json:"request_count_24h"`
	DiffCount24h    int64   `json:"diff_count_24h"`
	DiffRate        float64 `json:"diff_rate"`
	LastActive      *string `json:"last_active"`
}

// DiffConfig holds per-gate diff computation settings.
type DiffConfig struct {
	IgnoredFields  []string `json:"ignored_fields"`
	IncludedFields []string `json:"included_fields"`
	FloatTolerance float64  `json:"float_tolerance"`
	SortArrays     bool     `json:"sort_arrays"`
}

// Gate represents a traffic testing gate with live and shadow URLs.
type Gate struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	LiveURL        string     `json:"live_url"`
	ShadowURL      string     `json:"shadow_url"`
	DiffConfig     DiffConfig `json:"diff_config"`
	RedactedFields []string   `json:"redacted_fields"`
	// Retention is a per-gate Go duration string (e.g. "168h"). An empty string
	// means the gate uses the global retention floor.
	Retention string `json:"retention"`
	// ShadowAdapter is null when requests are compared as is.
	ShadowAdapter *ShadowAdapter `json:"shadow_adapter"`
	CreatedAt     string         `json:"created_at"`
	Stats         GateStats      `json:"stats"`
}

// ShadowAdapter makes a shadow service that speaks a different protocol
// comparable with live (e.g. REST live vs GraphQL shadow).
type ShadowAdapter struct {
	// Type is the adapter type. Supported: "graphql".
	Type string `json:"type"`
	// Config is the adapter configuration document, e.g. the YAML mapping.
	Config string `json:"config"`
	// Version identifies the mapping (a hash of type and config). Read-only:
	// it is ignored on create and update.
	Version string `json:"version,omitempty"`
}
