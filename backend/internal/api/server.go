package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"logicstation/internal/decode"
	"logicstation/internal/model"
	"logicstation/internal/store"
)

type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

func NewServer(st *store.Store, staticDir string) http.Handler {
	server := &Server{store: st, mux: http.NewServeMux()}
	server.routes(staticDir)
	return logging(cors(server.mux))
}

func (s *Server) routes(staticDir string) {
	s.mux.HandleFunc("GET /api/protocols", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, decode.SupportedProtocols())
	})
	s.mux.HandleFunc("GET /api/captures", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.store.ListCaptures())
	})
	s.mux.HandleFunc("POST /api/captures", s.createCapture)
	s.mux.HandleFunc("POST /api/demo", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, CreateDemo(s.store))
	})
	s.mux.HandleFunc("GET /api/captures/{id}", s.getCapture)
	s.mux.HandleFunc("POST /api/captures/{id}/chunks", s.addChunks)
	s.mux.HandleFunc("GET /api/captures/{id}/configs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.store.ListConfigs(r.PathValue("id")))
	})
	s.mux.HandleFunc("POST /api/captures/{id}/configs", s.createConfig)
	s.mux.HandleFunc("GET /api/captures/{id}/results", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.store.ListResults(r.PathValue("id")))
	})
	s.mux.HandleFunc("POST /api/configs/{id}/recalculate", func(w http.ResponseWriter, r *http.Request) {
		result, err := s.store.Recalculate(r.PathValue("id"))
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, result)
	})
	s.mux.HandleFunc("GET /api/results/diff", s.diffResults)
	if staticDir != "" {
		s.mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	}
}

func (s *Server) diffResults(w http.ResponseWriter, r *http.Request) {
	aID, bID := r.URL.Query().Get("a"), r.URL.Query().Get("b")
	if aID == "" || bID == "" {
		writeError(w, http.StatusBadRequest, "需要 a 和 b 两个结果版本 ID")
		return
	}
	resultA, ok := s.store.GetResult(aID)
	if !ok {
		writeError(w, http.StatusNotFound, "result a not found")
		return
	}
	resultB, ok := s.store.GetResult(bID)
	if !ok {
		writeError(w, http.StatusNotFound, "result b not found")
		return
	}
	cfgA, _ := s.store.GetConfig(resultA.ConfigID)
	cfgB, _ := s.store.GetConfig(resultB.ConfigID)
	diff := decode.DiffResults(resultA, resultB, cfgA, cfgB)
	if diff == nil {
		writeJSON(w, map[string]any{"diff": nil, "message": "两个结果版本的字节序列完全一致"})
		return
	}
	writeJSON(w, map[string]any{"diff": diff})
}

func (s *Server) createCapture(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         string   `json:"name"`
		ChannelNames []string `json:"channelNames"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(in.ChannelNames) == 0 {
		in.ChannelNames = []string{"RX"}
	}
	writeJSON(w, s.store.CreateCapture(in.Name, in.ChannelNames))
}

func (s *Server) getCapture(w http.ResponseWriter, r *http.Request) {
	capture, chunks, ok := s.store.GetCapture(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}
	writeJSON(w, map[string]any{"capture": capture, "chunks": chunks})
}

func (s *Server) addChunks(w http.ResponseWriter, r *http.Request) {
	var chunks []model.Chunk
	if err := json.NewDecoder(r.Body).Decode(&chunks); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for i := range chunks {
		chunks[i].CaptureID = r.PathValue("id")
	}
	saved, err := s.store.AddChunks(chunks)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, saved)
}

func (s *Server) createConfig(w http.ResponseWriter, r *http.Request) {
	var cfg model.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg.CaptureID = r.PathValue("id")
	if cfg.Protocol == "" {
		cfg.Protocol = "uart"
	}
	saved, err := s.store.CreateConfig(cfg)
	if err != nil {
		httpError(w, err)
		return
	}
	result, err := s.store.Recalculate(saved.ID)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, map[string]any{"config": saved, "result": result})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	writeJSON(w, map[string]string{"error": message})
}

func httpError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "unknown") || strings.Contains(err.Error(), "not found") {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.RequestURI())
		next.ServeHTTP(w, r)
	})
}
