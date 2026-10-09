package traffictesting

import (
	"reflect"

	"github.com/pedrobarco/mroki/pkg/diff"
)

type Diff struct {
	Content []diff.PatchOp
	Config  DiffConfig
	// ShadowAdapter records the adapter and mapping version the diff was
	// computed with. When set, only the (normalized) bodies were compared.
	ShadowAdapter ShadowAdapterSnapshot
}

type diffOption func(*Diff)

// WithDiffShadowAdapter records the shadow adapter the diff was computed with.
func WithDiffShadowAdapter(s ShadowAdapterSnapshot) diffOption {
	return func(d *Diff) {
		d.ShadowAdapter = s
	}
}

func NewDiff(content []diff.PatchOp, config DiffConfig, opts ...diffOption) (*Diff, error) {
	d := &Diff{
		Content: content,
		Config:  config,
	}

	for _, o := range opts {
		o(d)
	}

	return d, nil
}

// IsZero returns true if the Diff is the zero value (no diff present).
// A Diff created via NewDiff always has non-nil Content, so a nil Content
// distinguishes the zero value from an empty (identical-response) diff.
func (d Diff) IsZero() bool {
	return d.Content == nil
}

// HasContent returns true if the Diff contains actual differences.
// A Diff with empty Content means the compared responses were identical.
func (d Diff) HasContent() bool {
	return len(d.Content) > 0
}

// Equals checks value equality by comparing Content, Config and ShadowAdapter.
func (d Diff) Equals(other Diff) bool {
	if !reflect.DeepEqual(d.Content, other.Content) {
		return false
	}
	if d.ShadowAdapter != other.ShadowAdapter {
		return false
	}
	return reflect.DeepEqual(d.Config, other.Config)
}
