package graphql

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/pedrobarco/mroki/pkg/jsontree"
)

// ConversionType names the type a GraphQL field value is converted to before
// it is compared with the REST value. The zero value means no conversion.
type ConversionType string

const (
	// AsNumber converts numeric text such as "1042" or "450.50" to a number.
	AsNumber ConversionType = "number"
	// AsString converts a number or boolean to text, e.g. 1042 to "1042".
	AsString ConversionType = "string"
	// AsBoolean converts the texts listed in BooleanTrueValues and
	// BooleanFalseValues (and the numbers 1 and 0) to a boolean.
	AsBoolean ConversionType = "boolean"
	// AsDate converts a date ("2026-10-09") or a midnight date-time in UTC or
	// without a zone ("2026-10-09T00:00:00Z") to a date ("2026-10-09").
	AsDate ConversionType = "date"
)

// conversionTypes lists the supported types, in the order shown in errors.
var conversionTypes = []ConversionType{AsNumber, AsString, AsBoolean, AsDate}

// BooleanTrueValues and BooleanFalseValues are the texts accepted by
// AsBoolean, compared case-insensitively. Any other text fails to convert.
var (
	BooleanTrueValues  = []string{"true", "t", "1", "s", "si", "sí", "y", "yes"}
	BooleanFalseValues = []string{"false", "f", "0", "n", "no"}
)

// jsonNumber matches a number written as JSON writes it: no spaces, no "+",
// no leading zeros, no thousands separators, no hexadecimal.
var jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// dateLayout is the date format a converted date has, as in the REST response.
const dateLayout = "2006-01-02"

// FieldMapping says where a REST field's value comes from in the GraphQL
// response and, optionally, the type it is converted to before comparing.
// In YAML it is either the GraphQL field path ("guestName") or an object
// ("{from: id, as: number}").
type FieldMapping struct {
	// From is the GraphQL field path (dot-separated, relative to the root).
	From string `yaml:"from"`
	// As is the type the GraphQL value is converted to; empty for none.
	As ConversionType `yaml:"as"`
}

// UnmarshalYAML accepts both forms of a field mapping. Unknown keys in the
// object form are rejected, like everywhere else in the mapping.
func (f *FieldMapping) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*f = FieldMapping{}
		return node.Decode(&f.From)
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			if key := node.Content[i].Value; key != "from" && key != "as" {
				return fmt.Errorf("line %d: unknown key %q in field mapping (expected \"from\" and \"as\")", node.Content[i].Line, key)
			}
		}
		type plain FieldMapping // without the UnmarshalYAML method
		var p plain
		if err := node.Decode(&p); err != nil {
			return err
		}
		*f = FieldMapping(p)
		return nil
	default:
		return fmt.Errorf("line %d: a field mapping must be a GraphQL field name or {from: <field>, as: <type>}", node.Line)
	}
}

// validate checks the conversion type, if any.
func (t ConversionType) validate() error {
	if t == "" {
		return nil
	}
	for _, known := range conversionTypes {
		if t == known {
			return nil
		}
	}
	names := make([]string, len(conversionTypes))
	for i, known := range conversionTypes {
		names[i] = string(known)
	}
	return fmt.Errorf("unsupported type %q (supported: %s)", string(t), strings.Join(names, ", "))
}

// Conversion records a declared conversion applied to one field of a shadow
// response, so the comparison can show which values were compared converted.
type Conversion struct {
	// Field is the REST field path (dot-separated), e.g. "id".
	Field string
	// As is the type the value was converted to.
	As ConversionType
	// Original is the GraphQL value as it was received.
	Original jsontree.Tree
	// Error says why the value could not be converted; empty on success. A
	// value that fails keeps its original form, so the diff shows it.
	Error string
}

// OK reports whether the value was converted.
func (c Conversion) OK() bool {
	return c.Error == ""
}

// convert converts a JSON value to the type. JSON null is not converted
// (callers skip it), so every other value either converts or fails.
func (t ConversionType) convert(v jsontree.Tree) (jsontree.Tree, error) {
	switch t {
	case AsNumber:
		return toNumber(v)
	case AsString:
		return toString(v)
	case AsBoolean:
		return toBoolean(v)
	case AsDate:
		return toDate(v)
	default:
		return nil, fmt.Errorf("unsupported type %q", string(t))
	}
}

func toNumber(v jsontree.Tree) (jsontree.Tree, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case string:
		if !jsonNumber.MatchString(x) {
			return nil, fmt.Errorf("%q is not a number", x)
		}
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is out of range for a number", x)
		}
		return f, nil
	default:
		return nil, fmt.Errorf("%s is not a number", describe(v))
	}
}

func toString(v jsontree.Tree) (jsontree.Tree, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(x), nil
	default:
		return nil, fmt.Errorf("%s cannot be converted to text", describe(v))
	}
}

func toBoolean(v jsontree.Tree) (jsontree.Tree, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case float64:
		switch x {
		case 1:
			return true, nil
		case 0:
			return false, nil
		}
		return nil, fmt.Errorf("%v is not a boolean (only 1 and 0 are)", x)
	case string:
		for _, s := range BooleanTrueValues {
			if strings.EqualFold(x, s) {
				return true, nil
			}
		}
		for _, s := range BooleanFalseValues {
			if strings.EqualFold(x, s) {
				return false, nil
			}
		}
		return nil, fmt.Errorf("%q is not a boolean", x)
	default:
		return nil, fmt.Errorf("%s is not a boolean", describe(v))
	}
}

// toDate accepts "2026-10-09", or a date-time at exactly midnight that is in
// UTC ("Z") or has no zone. Any other time or offset fails: a date shifted by
// a time zone is a difference worth seeing.
func toDate(v jsontree.Tree) (jsontree.Tree, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("%s is not a date", describe(v))
	}
	if _, err := time.Parse(dateLayout, s); err == nil {
		return s, nil
	}

	var (
		t   time.Time
		err error
	)
	if strings.HasSuffix(s, "Z") {
		t, err = time.Parse(time.RFC3339Nano, s)
	} else {
		t, err = time.Parse("2006-01-02T15:04:05.999999999", s)
		if err != nil {
			// A date-time with an offset other than "Z" parses as RFC 3339.
			if withOffset, err2 := time.Parse(time.RFC3339Nano, s); err2 == nil {
				return nil, fmt.Errorf("%q has offset %s, not UTC (Z)", s, withOffset.Format("-07:00"))
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%q is not a date", s)
	}
	if t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0 || t.Nanosecond() != 0 {
		return nil, fmt.Errorf("%q is not at midnight", s)
	}
	return t.Format(dateLayout), nil
}

// describe names a JSON value's kind for error messages.
func describe(v jsontree.Tree) string {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	default:
		return fmt.Sprintf("%v", x)
	}
}
