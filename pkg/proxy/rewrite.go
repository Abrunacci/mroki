package proxy

import "net/http"

// ShadowRequest describes the request sent to the shadow target. Path is joined
// onto the shadow URL's path and RawQuery is merged with its query, exactly as
// for an unmodified shadow request.
type ShadowRequest struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// ShadowRequestRewriter builds the shadow request from the original request and
// its buffered body. It lets the shadow target speak a different protocol than
// live (e.g. a REST request translated into a GraphQL query). Implementations
// must treat r and body as read-only.
//
// Returning an error skips shadow for that request: live traffic is forwarded
// as usual and no comparison runs.
type ShadowRequestRewriter func(r *http.Request, body []byte) (ShadowRequest, error)

// WithShadowRequestRewriter sets a function that rewrites the request sent to
// shadow. When unset, shadow receives a copy of the live request.
func WithShadowRequestRewriter(fn ShadowRequestRewriter) Option {
	return func(p *Proxy) {
		p.shadowRewriter = fn
	}
}
