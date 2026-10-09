// Command setup creates (or updates) the example's gate through the mroki-api
// REST API, the same way a person would with curl: it reads mapping.yaml and
// sends it as the gate's shadow_adapter. It then writes the gate ID to a file
// so mroki-proxy can start in API mode with it.
//
// Running it again updates the gate's mapping (PATCH), so an edited
// mapping.yaml shows up in the hub as a new mapping version.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

// shadowAdapter is the gate's shadow_adapter field: the adapter type and the
// mapping document, sent as is.
type shadowAdapter struct {
	Type   string `json:"type"`
	Config string `json:"config"`
}

type gate struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	ShadowAdapter *shadowAdapter `json:"shadow_adapter,omitempty"`
}

type apiClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func main() {
	apiURL := flag.String("api-url", envOr("MROKI_API_URL", "http://localhost:8090"), "mroki-api base URL")
	apiKey := flag.String("api-key", os.Getenv("MROKI_API_KEY"), "mroki-api key")
	name := flag.String("name", "bookings-rest-to-graphql", "gate name")
	liveURL := flag.String("live-url", "http://legacy:9001", "live (legacy REST) URL, as seen by mroki-proxy")
	shadowURL := flag.String("shadow-url", "http://graphql:9002", "shadow (GraphQL) URL, as seen by mroki-proxy")
	mappingFile := flag.String("mapping", "mapping.yaml", "REST → GraphQL mapping file")
	gateIDFile := flag.String("gate-id-file", "", "file to write the gate ID to (optional)")
	flag.Parse()

	mapping, err := os.ReadFile(*mappingFile)
	if err != nil {
		log.Fatalf("read mapping: %v", err)
	}
	adapter := &shadowAdapter{Type: "graphql", Config: string(mapping)}

	c := apiClient{baseURL: *apiURL, apiKey: *apiKey, http: &http.Client{Timeout: 10 * time.Second}}
	if err := c.waitReady(60 * time.Second); err != nil {
		log.Fatal(err)
	}

	g, err := c.findGate(*name)
	if err != nil {
		log.Fatal(err)
	}
	if g == nil {
		// POST /gates: the gate does not exist yet.
		g, err = c.send(http.MethodPost, "/gates", map[string]any{
			"name":           *name,
			"live_url":       *liveURL,
			"shadow_url":     *shadowURL,
			"shadow_adapter": adapter,
		}, http.StatusCreated)
		if err != nil {
			log.Fatalf("create gate: %v", err)
		}
		log.Printf("created gate %q (%s)", g.Name, g.ID)
	} else {
		// PATCH /gates/{id}: keep the gate (and its history), refresh the mapping.
		g, err = c.send(http.MethodPatch, "/gates/"+g.ID, map[string]any{
			"shadow_adapter": adapter,
		}, http.StatusOK)
		if err != nil {
			log.Fatalf("update gate: %v", err)
		}
		log.Printf("updated the mapping of gate %q (%s)", g.Name, g.ID)
	}

	if *gateIDFile != "" {
		if err := os.WriteFile(*gateIDFile, []byte(g.ID), 0o644); err != nil {
			log.Fatalf("write gate ID: %v", err)
		}
	}
	fmt.Println(g.ID)
}

// waitReady polls GET /health/ready until mroki-api answers 200.
func (c apiClient) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := c.http.Get(c.baseURL + "/health/ready")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("mroki-api at %s is not ready after %s", c.baseURL, timeout)
		}
		time.Sleep(time.Second)
	}
}

// findGate returns the gate with exactly this name, or nil. The API's name
// filter is a substring match, so the exact name is checked here.
func (c apiClient) findGate(name string) (*gate, error) {
	req, err := c.request(http.MethodGet, "/gates?limit=100&name="+url.QueryEscape(name), nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		Data []gate `json:"data"`
	}
	if err := c.do(req, http.StatusOK, &page); err != nil {
		return nil, fmt.Errorf("list gates: %w", err)
	}
	for _, g := range page.Data {
		if g.Name == name {
			return &g, nil
		}
	}
	return nil, nil
}

func (c apiClient) send(method, path string, body any, wantStatus int) (*gate, error) {
	req, err := c.request(method, path, body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data gate `json:"data"`
	}
	if err := c.do(req, wantStatus, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c apiClient) request(method, path string, body any) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// do sends the request and decodes the JSON response into out. Any other
// status than wantStatus is an error that carries the API's problem detail.
func (c apiClient) do(req *http.Request, wantStatus int, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != wantStatus {
		var problem struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(b, &problem)
		if problem.Detail == "" {
			problem.Detail = string(b)
		}
		return errors.New(resp.Status + ": " + problem.Detail)
	}
	return json.Unmarshal(b, out)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
