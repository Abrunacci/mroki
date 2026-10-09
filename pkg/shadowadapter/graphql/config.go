// Package graphql lets mroki compare a REST live service against a GraphQL
// shadow service. It translates matching REST requests into GraphQL queries
// and normalizes GraphQL responses back into the REST shape, so the regular
// diff pipeline compares like with like.
package graphql

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ErrInvalidConfig is returned when a mapping configuration fails validation.
var ErrInvalidConfig = errors.New("invalid graphql mapping config")

// pathVariablePrefix marks a variable whose value comes from a path parameter
// of the matched route, e.g. "path.id" for "GET /bookings/{id}".
const pathVariablePrefix = "path."

// Config is the REST → GraphQL mapping, usually loaded from a YAML file.
//
//	endpoint: /graphql
//	routes:
//	  - match: GET /bookings/{id}
//	    query: |
//	      query GetBooking($id: ID!) { booking(id: $id) { id guestName } }
//	    variables:
//	      id: path.id
//	    response:
//	      root: data.booking
//	      fields:
//	        id: { from: id, as: number }
//	        guest_name: guestName
type Config struct {
	// Endpoint is the GraphQL path on the shadow URL (e.g. "/graphql").
	Endpoint string `yaml:"endpoint"`
	// Routes lists the REST routes that are translated. Requests that match no
	// route are not shadowed.
	Routes []RouteConfig `yaml:"routes"`
}

// RouteConfig maps one REST route to a GraphQL query.
type RouteConfig struct {
	// Match is an http.ServeMux pattern with a method, e.g. "GET /bookings/{id}".
	Match string `yaml:"match"`
	// Query is the GraphQL query document sent to the shadow service.
	Query string `yaml:"query"`
	// Variables maps each GraphQL variable name to its source. Only path
	// parameters are supported ("path.<name>").
	Variables map[string]string `yaml:"variables"`
	// Response describes how to normalize the GraphQL response.
	Response ResponseConfig `yaml:"response"`
}

// ResponseConfig describes how a GraphQL response is turned into the REST shape.
type ResponseConfig struct {
	// Root is the dot-separated path of the object to compare, e.g. "data.booking".
	Root string `yaml:"root"`
	// Fields maps each REST field (dot path) to the GraphQL field (dot path,
	// relative to Root) that holds its value, optionally with the type the
	// GraphQL value is converted to before comparing. Only mapped fields are
	// compared, on both sides.
	Fields map[string]FieldMapping `yaml:"fields"`
}

// LoadConfig reads and validates a mapping configuration from a YAML file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read graphql mapping config: %w", err)
	}
	return ParseConfig(data)
}

// ParseConfig parses and validates a YAML mapping configuration. Unknown keys
// are rejected so typos surface at startup instead of silently not matching.
func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks the configuration and returns the first problem found,
// wrapped in ErrInvalidConfig.
func (c Config) Validate() error {
	if !strings.HasPrefix(c.Endpoint, "/") {
		return fmt.Errorf("%w: endpoint must start with \"/\", got %q", ErrInvalidConfig, c.Endpoint)
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("%w: at least one route is required", ErrInvalidConfig)
	}
	for i, r := range c.Routes {
		if err := r.validate(); err != nil {
			return fmt.Errorf("%w: routes[%d] (%s): %v", ErrInvalidConfig, i, r.Match, err)
		}
	}
	return nil
}

func (r RouteConfig) validate() error {
	method, path, ok := strings.Cut(strings.TrimSpace(r.Match), " ")
	if !ok || method == "" || !strings.HasPrefix(strings.TrimSpace(path), "/") {
		return fmt.Errorf("match must be \"METHOD /path\", got %q", r.Match)
	}

	if strings.TrimSpace(r.Query) == "" {
		return errors.New("query must not be empty")
	}
	if op := operationType(r.Query); op != "query" {
		return fmt.Errorf("only queries are supported, got a %s", op)
	}

	for name, source := range r.Variables {
		param, ok := strings.CutPrefix(source, pathVariablePrefix)
		if !ok || param == "" {
			return fmt.Errorf("variable %q: unsupported source %q (only %q<name> is supported)", name, source, pathVariablePrefix)
		}
		if !strings.Contains(r.Match, "{"+param+"}") {
			return fmt.Errorf("variable %q: path parameter {%s} is not in the route pattern", name, param)
		}
	}

	if err := validateDotPath(r.Response.Root); err != nil {
		return fmt.Errorf("response.root: %w", err)
	}
	if len(r.Response.Fields) == 0 {
		return errors.New("response.fields must map at least one field")
	}
	for rest, f := range r.Response.Fields {
		if err := validateDotPath(rest); err != nil {
			return fmt.Errorf("response.fields key %q: %w", rest, err)
		}
		if err := validateDotPath(f.From); err != nil {
			return fmt.Errorf("response.fields[%q]: %w", rest, err)
		}
		if err := f.As.validate(); err != nil {
			return fmt.Errorf("response.fields[%q].as: %w", rest, err)
		}
	}
	return nil
}

// operationType returns the GraphQL operation type of a query document:
// "query", "mutation" or "subscription". The shorthand "{ ... }" is a query.
// Comment lines (starting with "#") are skipped.
func operationType(query string) string {
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, op := range []string{"mutation", "subscription"} {
			if line == op || strings.HasPrefix(line, op+" ") || strings.HasPrefix(line, op+"{") || strings.HasPrefix(line, op+"(") {
				return op
			}
		}
		return "query"
	}
	return "query"
}

// validateDotPath checks a dot-separated field path such as "data.booking".
func validateDotPath(path string) error {
	if path == "" {
		return errors.New("must not be empty")
	}
	for _, seg := range strings.Split(path, ".") {
		if seg == "" {
			return fmt.Errorf("invalid path %q: empty segment", path)
		}
		if seg == "#" {
			return fmt.Errorf("invalid path %q: array wildcards are not supported yet", path)
		}
	}
	return nil
}
