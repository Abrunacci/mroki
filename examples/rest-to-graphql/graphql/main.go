// Command graphql is a minimal stand-in for the new GraphQL bookings service,
// used by the rest-to-graphql example as mroki's shadow target. It is not a
// real GraphQL server: it answers the single booking(id) query of the example.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

// booking is the GraphQL representation: camelCase fields, ID as a string and
// dates as RFC 3339 timestamps.
type booking struct {
	ID         string  `json:"id"`
	GuestName  string  `json:"guestName"`
	CheckIn    string  `json:"checkIn"`
	CheckOut   string  `json:"checkOut"`
	Status     string  `json:"status"`
	TotalPrice float64 `json:"totalPrice"`
}

var bookings = map[string]booking{
	// 1042: the id and check-in types differ (accepted by the mapping's
	// conversions), and the price differs for real: a migration bug.
	"1042": {ID: "1042", GuestName: "Ana Pérez", CheckIn: "2026-10-09T00:00:00Z", CheckOut: "2026-10-12", Status: "confirmed", TotalPrice: 405.5},
	// 1043: only the accepted type differences, so no differences remain.
	"1043": {ID: "1043", GuestName: "Luis Gómez", CheckIn: "2026-11-02T00:00:00Z", CheckOut: "2026-11-04", Status: "cancelled", TotalPrice: 180},
	// 1044: a Relay-style global ID, which cannot be compared as a number.
	"1044": {ID: "Qm9va2luZzoxMDQ0", GuestName: "Marta Ruiz", CheckIn: "2026-12-20T00:00:00Z", CheckOut: "2026-12-23", Status: "confirmed", TotalPrice: 320},
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Data   map[string]any `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

func main() {
	addr := flag.String("addr", ":9002", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var req graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !strings.Contains(req.Query, "booking(") {
			_ = json.NewEncoder(w).Encode(graphQLResponse{Errors: []graphQLError{{Message: "unsupported query"}}})
			return
		}

		id, _ := req.Variables["id"].(string)
		b, ok := bookings[id]
		if !ok {
			// GraphQL reports "not found" as null data plus an error, with HTTP 200.
			_ = json.NewEncoder(w).Encode(graphQLResponse{
				Data:   map[string]any{"booking": nil},
				Errors: []graphQLError{{Message: "booking not found"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(graphQLResponse{Data: map[string]any{"booking": b}})
	})

	log.Printf("GraphQL bookings service listening on %s", *addr)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
