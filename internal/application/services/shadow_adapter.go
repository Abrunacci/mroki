package services

import (
	"fmt"
	"sync"

	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
	"github.com/pedrobarco/mroki/pkg/shadowadapter/graphql"
)

// RouteNormalizer normalizes the live and shadow bodies of one request,
// selected by its method and path. It is satisfied by *graphql.Adapter.
type RouteNormalizer interface {
	Normalize(method, path string, live, shadow []byte) ([]byte, []byte, error)
}

// ShadowAdapterOptions returns the ResponseComparer options for a request
// handled by a shadow adapter: bodies are normalized for the request's route
// and only bodies are compared, since status codes and headers differ by
// protocol. onError (optional) is called when normalization fails; the
// original bodies are then compared instead.
func ShadowAdapterOptions(n RouteNormalizer, method, path string, onError func(error)) []ComparerOption {
	normalize := func(live, shadow []byte) ([]byte, []byte, error) {
		l, s, err := n.Normalize(method, path, live, shadow)
		if err != nil && onError != nil {
			onError(err)
		}
		return l, s, err
	}
	return []ComparerOption{WithBodyNormalizer(normalize), WithBodyOnly()}
}

// maxCachedShadowAdapters bounds the ShadowAdapterCache. Entries are keyed by
// mapping version, so the cache only grows when mappings are edited; when the
// bound is reached the cache is simply reset.
const maxCachedShadowAdapters = 64

// ShadowAdapterCache compiles gate shadow adapters once and reuses them across
// requests, keyed by the adapter version (a hash of type and config), so an
// edited mapping is picked up on the next request without invalidation.
// It is safe for concurrent use.
type ShadowAdapterCache struct {
	mu       sync.Mutex
	adapters map[string]RouteNormalizer
}

// NewShadowAdapterCache returns an empty ShadowAdapterCache.
func NewShadowAdapterCache() *ShadowAdapterCache {
	return &ShadowAdapterCache{adapters: make(map[string]RouteNormalizer)}
}

// Get returns the compiled normalizer for a gate's shadow adapter.
func (c *ShadowAdapterCache) Get(a traffictesting.ShadowAdapter) (RouteNormalizer, error) {
	if !a.IsSet() {
		return nil, fmt.Errorf("%w: not set", traffictesting.ErrInvalidShadowAdapter)
	}
	key := a.Version()

	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.adapters[key]; ok {
		return n, nil
	}

	n, err := compileShadowAdapter(a)
	if err != nil {
		return nil, err
	}
	if len(c.adapters) >= maxCachedShadowAdapters {
		c.adapters = make(map[string]RouteNormalizer)
	}
	c.adapters[key] = n
	return n, nil
}

func compileShadowAdapter(a traffictesting.ShadowAdapter) (RouteNormalizer, error) {
	switch a.Type() {
	case traffictesting.ShadowAdapterGraphQL:
		cfg, err := graphql.ParseConfig([]byte(a.Config()))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", traffictesting.ErrInvalidShadowAdapter, err)
		}
		adapter, err := graphql.New(cfg)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", traffictesting.ErrInvalidShadowAdapter, err)
		}
		return adapter, nil
	default:
		return nil, fmt.Errorf("%w: unsupported type %q", traffictesting.ErrInvalidShadowAdapter, a.Type())
	}
}
