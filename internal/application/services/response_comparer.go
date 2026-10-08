package services

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
	"github.com/pedrobarco/mroki/pkg/diff"
)

// ResponseData holds the raw inputs for one side of the comparison.
type ResponseData struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// CompareResult holds all redacted results and the computed diff ops.
type CompareResult struct {
	Request traffictesting.RedactResult
	Live    traffictesting.RedactResult
	Shadow  traffictesting.RedactResult
	Ops     []diff.PatchOp
}

// BodyNormalizer rewrites the live and shadow bodies into a comparable shape
// before they are redacted and diffed, e.g. a GraphQL response into the shape
// of the REST response it replaces.
type BodyNormalizer func(live, shadow []byte) ([]byte, []byte, error)

// ResponseComparer encapsulates the normalize + redact + envelope + diff pipeline.
type ResponseComparer struct {
	redactor   *traffictesting.Redactor
	diffOpts   []diff.Option
	normalizer BodyNormalizer
	bodyOnly   bool
}

// ComparerOption configures optional steps of a ResponseComparer.
type ComparerOption func(*ResponseComparer)

// WithBodyNormalizer runs fn on the live and shadow bodies before redaction, so
// redaction and diff field paths apply to the normalized shape on both sides.
// A normalization error is non-fatal: the original bodies are compared, and fn
// is responsible for reporting the error.
func WithBodyNormalizer(fn BodyNormalizer) ComparerOption {
	return func(c *ResponseComparer) {
		c.normalizer = fn
	}
}

// WithBodyOnly compares only the bodies, leaving status codes and headers out
// of the diff. Useful when live and shadow speak different protocols (e.g.
// GraphQL answers 200 with an error where REST answers 404).
func WithBodyOnly() ComparerOption {
	return func(c *ResponseComparer) {
		c.bodyOnly = true
	}
}

// NewResponseComparer creates a ResponseComparer with the given redactor and diff options.
func NewResponseComparer(redactor *traffictesting.Redactor, diffOpts []diff.Option, opts ...ComparerOption) *ResponseComparer {
	c := &ResponseComparer{
		redactor: redactor,
		diffOpts: diffOpts,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ErrNilRedactor is returned when Compare is called with a nil redactor.
var ErrNilRedactor = errors.New("redactor must not be nil")

// Compare normalizes the bodies (when configured), redacts all three inputs,
// builds envelopes from live and shadow, and computes the diff between them.
//
// Redaction errors are fatal (returned immediately).
// Diff errors are non-fatal (ops defaults to empty slice).
func (c *ResponseComparer) Compare(req, live, shadow ResponseData) (*CompareResult, error) {
	if c.redactor == nil {
		return nil, ErrNilRedactor
	}

	// 1. Normalize the bodies into a comparable shape, when configured. A
	// failure falls back to the original bodies (best-effort).
	if c.normalizer != nil {
		if liveBody, shadowBody, err := c.normalizer(live.Body, shadow.Body); err == nil {
			live.Body = liveBody
			shadow.Body = shadowBody
		}
	}

	// 2. Redact all three inputs.
	reqResult, err := c.redactor.Redact(req.Headers, req.Body)
	if err != nil {
		return nil, fmt.Errorf("request redaction: %w", err)
	}

	liveResult, err := c.redactor.Redact(live.Headers, live.Body)
	if err != nil {
		return nil, fmt.Errorf("live response redaction: %w", err)
	}

	shadowResult, err := c.redactor.Redact(shadow.Headers, shadow.Body)
	if err != nil {
		return nil, fmt.Errorf("shadow response redaction: %w", err)
	}

	// 3. Embed each body per its Content-Type (JSON tree, raw text, or binary
	// note) so differences surface for every content type, not just JSON.
	liveBody := diff.EmbedBody(live.Headers.Get("Content-Type"), liveResult.Body, liveResult.BodyParsed)
	shadowBody := diff.EmbedBody(shadow.Headers.Get("Content-Type"), shadowResult.Body, shadowResult.BodyParsed)

	var liveEnvelope, shadowEnvelope map[string]any
	if c.bodyOnly {
		liveEnvelope = diff.BuildEnvelope(0, nil, liveBody)
		shadowEnvelope = diff.BuildEnvelope(0, nil, shadowBody)
	} else {
		liveEnvelope = diff.BuildEnvelope(live.StatusCode, liveResult.Headers, liveBody)
		shadowEnvelope = diff.BuildEnvelope(shadow.StatusCode, shadowResult.Headers, shadowBody)
	}

	// 4. Compute diff — errors are non-fatal.
	ops, err := diff.Parsed(liveEnvelope, shadowEnvelope, c.diffOpts...)
	if err != nil {
		ops = []diff.PatchOp{}
	}

	return &CompareResult{
		Request: reqResult,
		Live:    liveResult,
		Shadow:  shadowResult,
		Ops:     ops,
	}, nil
}
