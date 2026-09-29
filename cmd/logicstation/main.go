// Command logicstation serves the hardware logic analysis workstation API and
// the built React frontend.
package main

import (
	"flag"
	"log"
	"net/http"

	"logicstation/internal/api"
	"logicstation/internal/engine"
	"logicstation/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	static := flag.String("static", "frontend/dist", "built frontend directory")
	flag.Parse()

	st := store.New()
	eng := engine.New(st)
	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewServer(st, eng))
	mux.Handle("/", spaHandler(*static))

	log.Printf("logic station listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// spaHandler serves built assets and falls back to index.html for client-side
// routes. Missing asset directories simply 404 (API still works without a
// built frontend; use the Vite dev server during development).
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && fileExists(dir+r.URL.Path) {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, dir+"/index.html")
	})
}
