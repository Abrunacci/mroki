package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
	"github.com/pedrobarco/mroki/pkg/diff"
)

// DiffConfigSnapshot is the storage representation of a gate's diff
// configuration at the time a diff was computed. Stored as a JSON column
// so the frontend can interpret patch indices without consulting the
// (possibly changed) current gate config.
type DiffConfigSnapshot struct {
	SortArrays     bool     `json:"sort_arrays"`
	IgnoredFields  []string `json:"ignored_fields,omitempty"`
	IncludedFields []string `json:"included_fields,omitempty"`
	FloatTolerance float64  `json:"float_tolerance,omitempty"`
	// ShadowAdapter records the adapter and mapping version the diff was
	// computed with (nil when none). When set, only bodies were compared.
	ShadowAdapter *ShadowAdapterSnapshot `json:"shadow_adapter,omitempty"`
}

// ShadowAdapterSnapshot identifies a shadow adapter mapping by type and
// content version, and records the declared field conversions it applied.
type ShadowAdapterSnapshot struct {
	Type        string                    `json:"type"`
	Version     string                    `json:"version"`
	Conversions []FieldConversionSnapshot `json:"conversions,omitempty"`
}

// FieldConversionSnapshot records a declared type conversion applied to one
// field of the shadow body before diffing.
type FieldConversionSnapshot struct {
	Field    string          `json:"field"`
	As       string          `json:"as"`
	Original json.RawMessage `json:"original"`
	Error    string          `json:"error,omitempty"`
}

// Diff holds the schema definition for the Diff entity.
type Diff struct {
	ent.Schema
}

// Fields of the Diff.
func (Diff) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("request_id", uuid.UUID{}).
			Unique(),
		field.UUID("from_response_id", uuid.UUID{}),
		field.UUID("to_response_id", uuid.UUID{}),
		field.JSON("content", []diff.PatchOp{}),
		// has_content materializes "the diff has at least one patch op" as a
		// persisted boolean so the has_diff filter and diff stats avoid a
		// per-row JSON array-length computation. Set explicitly from the
		// domain on write (see request_repository.saveDiff); backfilled by
		// migration for existing rows.
		field.Bool("has_content"),
		field.JSON("config", DiffConfigSnapshot{}).
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the Diff.
func (Diff) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("request", Request.Type).
			Ref("diff").
			Field("request_id").
			Unique().
			Required(),
		edge.From("from_response", Response.Type).
			Ref("diffs_from").
			Field("from_response_id").
			Unique().
			Required(),
		edge.From("to_response", Response.Type).
			Ref("diffs_to").
			Field("to_response_id").
			Unique().
			Required(),
	}
}
