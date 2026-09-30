package main

import (
	"flag"
	"log"
	"net/http"

	"logicstation/internal/api"
	"logicstation/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	static := flag.String("static", "frontend/dist", "static file directory")
	data := flag.String("data", "logicstation-data.json", "snapshot file for save/reload (empty disables)")
	flag.Parse()
	handler := api.NewServer(store.New(*data), *static)
	log.Printf("logic station listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
