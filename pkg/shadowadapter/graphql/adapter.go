package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/pedrobarco/mroki/pkg/jsontree"
	"github.com/pedrobarco/mroki/pkg/proxy"
)

// ErrNoRoute is returned when a request matches none of the configured routes.
var ErrNoRoute = errors.New("no graphql route matches the request")

// Adapter translates REST requests into GraphQL queries and normalizes the
// GraphQL responses back into the REST shape. It is safe for concurrent use.
type Adapter struct {
	endpoint string
	mux      *http.ServeMux
}

// route is the compiled form of a RouteConfig.
type route struct {
	query string
	// variables maps each GraphQL variable name to a path parameter name.
	variables map[string]string
	root      string
	// fields is sorted by REST path so normalization is deterministic.
	fields []fieldMapping
}

type fieldMapping struct {
	rest    string
	graphql string
	as      ConversionType
}

// graphQLRequest is the JSON body of a GraphQL-over-HTTP POST request.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// New builds an Adapter from a configuration. The configuration is validated
// first, and conflicting route patterns are reported as an error.
func New(cfg Config) (*Adapter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	a := &Adapter{endpoint: cfg.Endpoint, mux: http.NewServeMux()}
	for i, rc := range cfg.Routes {
		if err := a.register(rc); err != nil {
			return nil, fmt.Errorf("%w: routes[%d] (%s): %v", ErrInvalidConfig, i, rc.Match, err)
		}
	}
	return a, nil
}

// register compiles a route and adds it to the mux. http.ServeMux panics on
// invalid or conflicting patterns, so the panic is turned into an error.
func (a *Adapter) register(rc RouteConfig) (err error) {
	r := &route{
		query:     rc.Query,
		variables: make(map[string]string, len(rc.Variables)),
		root:      rc.Response.Root,
	}
	for name, source := range rc.Variables {
		r.variables[name] = strings.TrimPrefix(source, pathVariablePrefix)
	}
	for rest, f := range rc.Response.Fields {
		r.fields = append(r.fields, fieldMapping{rest: rest, graphql: f.From, as: f.As})
	}
	sort.Slice(r.fields, func(i, j int) bool { return r.fields[i].rest < r.fields[j].rest })

	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()
	a.mux.Handle(strings.TrimSpace(rc.Match), routeHandler{route: r})
	return nil
}

// RewriteRequest translates a REST request into a GraphQL POST to the
// configured endpoint. It has the proxy.ShadowRequestRewriter signature, so it
// can be passed to proxy.WithShadowRequestRewriter. Requests that match no
// route return ErrNoRoute, which makes the proxy skip shadow.
func (a *Adapter) RewriteRequest(r *http.Request, _ []byte) (proxy.ShadowRequest, error) {
	m, ok := a.match(r.Method, r.URL.Path)
	if !ok {
		return proxy.ShadowRequest{}, fmt.Errorf("%w: %s %s", ErrNoRoute, r.Method, r.URL.Path)
	}

	vars := make(map[string]any, len(m.route.variables))
	for name, param := range m.route.variables {
		vars[name] = m.req.PathValue(param)
	}

	body, err := json.Marshal(graphQLRequest{Query: m.route.query, Variables: vars})
	if err != nil {
		return proxy.ShadowRequest{}, fmt.Errorf("encode graphql request: %w", err)
	}

	header := r.Header.Clone()
	if header == nil {
		header = http.Header{}
	}
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	header.Del("Content-Length")

	return proxy.ShadowRequest{
		Method: http.MethodPost,
		Path:   a.endpoint,
		Header: header,
		Body:   body,
	}, nil
}

// Normalized holds a live and a shadow body in a comparable shape, and the
// declared conversions applied to the shadow body.
type Normalized struct {
	Live   []byte
	Shadow []byte
	// Conversions lists, in REST field order, each mapped field with a
	// declared type whose GraphQL value was present and not null.
	Conversions []Conversion
}

// Normalize makes a live REST body and a shadow GraphQL body comparable for
// the route matching method and path:
//
//   - shadow: the object at the route's root is extracted and each mapped
//     GraphQL field is moved to its REST name, converted to its declared type
//     if it has one. A missing or null root becomes JSON null, so the
//     difference still shows up in the diff. A value that cannot be converted
//     is kept as received, so it shows up as a difference too.
//   - live: only the mapped REST fields are kept, so unmapped fields don't
//     produce noise.
//
// It returns ErrNoRoute when no route matches, and an error when either body is
// not valid JSON. Callers should then compare the original bodies.
func (a *Adapter) Normalize(method, path string, live, shadow []byte) (Normalized, error) {
	m, ok := a.match(method, path)
	if !ok {
		return Normalized{}, fmt.Errorf("%w: %s %s", ErrNoRoute, method, path)
	}

	var liveTree, shadowTree jsontree.Tree
	if err := json.Unmarshal(live, &liveTree); err != nil {
		return Normalized{}, fmt.Errorf("live body is not valid JSON: %w", err)
	}
	if err := json.Unmarshal(shadow, &shadowTree); err != nil {
		return Normalized{}, fmt.Errorf("shadow body is not valid JSON: %w", err)
	}

	restPaths := make([]string, len(m.route.fields))
	for i, f := range m.route.fields {
		restPaths[i] = f.rest
	}
	liveOut, err := json.Marshal(jsontree.PickPaths(liveTree, restPaths))
	if err != nil {
		return Normalized{}, fmt.Errorf("encode normalized live body: %w", err)
	}

	shadowNorm, conversions := m.route.normalizeShadow(shadowTree)
	shadowOut, err := json.Marshal(shadowNorm)
	if err != nil {
		return Normalized{}, fmt.Errorf("encode normalized shadow body: %w", err)
	}
	return Normalized{Live: liveOut, Shadow: shadowOut, Conversions: conversions}, nil
}

// normalizeShadow extracts the root object, renames its fields to REST names
// and applies the declared conversions, which it returns.
func (r *route) normalizeShadow(tree jsontree.Tree) (jsontree.Tree, []Conversion) {
	root, ok := lookup(tree, r.root)
	if !ok || root == nil {
		return nil, nil
	}
	out := make(map[string]any, len(r.fields))
	var conversions []Conversion
	for _, f := range r.fields {
		v, ok := lookup(root, f.graphql)
		if !ok {
			continue
		}
		if f.as != "" && v != nil {
			c := Conversion{Field: f.rest, As: f.as, Original: v}
			if converted, err := f.as.convert(v); err != nil {
				c.Error = err.Error()
			} else {
				v = converted
			}
			conversions = append(conversions, c)
		}
		setPath(out, strings.Split(f.rest, "."), v)
	}
	return out, conversions
}

// lookup returns the value at a dot-separated path and whether it exists.
func lookup(tree jsontree.Tree, path string) (jsontree.Tree, bool) {
	var (
		value jsontree.Tree
		found bool
	)
	jsontree.WalkPath(tree, path, func(parent map[string]any, key string) {
		value, found = parent[key]
	})
	return value, found
}

// setPath stores value at the given path, creating intermediate objects.
func setPath(dst map[string]any, segments []string, value jsontree.Tree) {
	for _, seg := range segments[:len(segments)-1] {
		next, ok := dst[seg].(map[string]any)
		if !ok {
			next = make(map[string]any)
			dst[seg] = next
		}
		dst = next
	}
	dst[segments[len(segments)-1]] = value
}

// match is the result of matching a request against the configured routes.
type match struct {
	route *route
	req   *http.Request
}

// matchKey is the context key under which routeHandler reports a match.
type matchKey struct{}

// match finds the route for a method and path, using the same matching rules
// as http.ServeMux (precedence, {name} wildcards). A fresh request is built so
// the caller's request is never mutated. HEAD requests never match: ServeMux
// lets a GET pattern match HEAD, but a HEAD response has no body to compare.
func (a *Adapter) match(method, path string) (match, bool) {
	var m match
	if method == http.MethodHead {
		return m, false
	}
	req := &http.Request{
		Method: method,
		URL:    &url.URL{Path: path},
		Header: http.Header{},
	}
	req = req.WithContext(context.WithValue(context.Background(), matchKey{}, &m))
	a.mux.ServeHTTP(discardResponseWriter{}, req)
	return m, m.route != nil
}

// routeHandler records the matched route and the request carrying the path
// values. ServeMux only exposes path values to the handler it dispatches to.
type routeHandler struct {
	route *route
}

func (h routeHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	if m, ok := r.Context().Value(matchKey{}).(*match); ok {
		m.route = h.route
		m.req = r
	}
}

// discardResponseWriter swallows the 404/405/redirect responses ServeMux writes
// when nothing matches.
type discardResponseWriter struct{}

func (discardResponseWriter) Header() http.Header         { return http.Header{} }
func (discardResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (discardResponseWriter) WriteHeader(int)             {}
