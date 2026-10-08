// Command legacy is a minimal stand-in for a legacy REST bookings service,
// used by the rest-to-graphql example as mroki's live target.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"time"
)

// booking is the legacy REST representation: snake_case fields, numeric id
// and plain dates.
type booking struct {
	ID         int     `json:"id"`
	GuestName  string  `json:"guest_name"`
	CheckIn    string  `json:"check_in"`
	CheckOut   string  `json:"check_out"`
	Status     string  `json:"status"`
	TotalPrice float64 `json:"total_price"`
	// LegacyCode exists only in the old system; it is not mapped, so mroki
	// leaves it out of the comparison.
	LegacyCode string `json:"legacy_code"`
}

var bookings = map[string]booking{
	"1042": {ID: 1042, GuestName: "Ana Pérez", CheckIn: "2026-10-09", CheckOut: "2026-10-12", Status: "confirmed", TotalPrice: 450.5, LegacyCode: "BK-1042"},
	"1043": {ID: 1043, GuestName: "Luis Gómez", CheckIn: "2026-11-02", CheckOut: "2026-11-04", Status: "cancelled", TotalPrice: 180, LegacyCode: "BK-1043"},
}

func main() {
	addr := flag.String("addr", ":9001", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /bookings/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, ok := bookings[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "booking not found"})
			return
		}
		_ = json.NewEncoder(w).Encode(b)
	})

	log.Printf("legacy REST bookings service listening on %s", *addr)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
