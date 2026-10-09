package traffictesting_test

import (
	"testing"

	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
	"github.com/stretchr/testify/assert"
)

func TestRedactor_RedactsBodyPath(t *testing.T) {
	r := traffictesting.NewRedactor([]string{"headers.Authorization", "body.card", "body.guest.document"})

	tests := []struct {
		path string
		want bool
	}{
		{path: "card", want: true},
		{path: "card.number", want: true},    // inside a redacted object
		{path: "guest", want: true},          // contains a redacted field
		{path: "guest.document", want: true}, // the redacted field itself
		{path: "guest.name", want: false},    // a sibling of the redacted field
		{path: "cardholder", want: false},    // same prefix, different field
		{path: "Authorization", want: false}, // header fields are not body fields
		{path: "id", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, r.RedactsBodyPath(tt.path))
		})
	}
}

func TestDiff_Equals_compares_conversions(t *testing.T) {
	a, _ := traffictesting.NewDiff(nil, traffictesting.DiffConfig{},
		traffictesting.WithDiffConversions([]traffictesting.FieldConversion{{Field: "id", As: "number", Original: []byte(`"1"`)}}))
	b, _ := traffictesting.NewDiff(nil, traffictesting.DiffConfig{})

	assert.True(t, a.Equals(*a))
	assert.False(t, a.Equals(*b))
}
