// Command traffic sends the example's sample requests through mroki-proxy, so
// the hub already has comparisons to show when it is opened.
package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"time"
)

// paths covers the cases the guide walks through: two existing bookings and
// one that does not exist (REST answers 404, GraphQL answers a null booking).
var paths = []string{"/bookings/1042", "/bookings/1043", "/bookings/9999"}

func main() {
	proxyURL := flag.String("proxy-url", "http://localhost:8080", "mroki-proxy URL")
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}
	for _, p := range paths {
		resp, err := client.Get(*proxyURL + p)
		if err != nil {
			log.Fatalf("GET %s: %v", p, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		log.Printf("GET %s -> %d %s", p, resp.StatusCode, body)
	}
	log.Printf("sent %d requests; open the hub to see the comparisons", len(paths))
}
