// Command checklists-server runs the Checklists HTTP server.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/mkandel/go-checklists/internal/api"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := api.NewMux()

	log.Printf("checklists-server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
