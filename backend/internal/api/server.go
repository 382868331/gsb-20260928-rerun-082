package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"logicstation/internal/align"
	"logicstation/internal/compare"
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
	s.mux.HandleFunc("POST /api/results/diff", s.diffResults)
	s.mux.HandleFunc("GET /api/alignments", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.store.ListAlignments())
	})
	s.mux.HandleFunc("POST /api/alignments", s.createAlignment)
	s.mux.HandleFunc("GET /api/alignments/{id}", s.getAlignment)
	s.mux.HandleFunc("POST /api/alignments/{id}/versions", s.addAlignmentVersion)
	s.mux.HandleFunc("POST /api/alignments/{id}/candidates", s.alignmentCandidates)
	s.mux.HandleFunc("POST /api/alignments/{id}/compare", s.compareAlignment)
	if staticDir != "" {
		s.mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	}
}

func (s *Server) createAlignment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name           string                  `json:"name"`
		BaseCaptureID  string                  `json:"baseCaptureId"`
		OtherCaptureID string                  `json:"otherCaptureId"`
		Anchors        []model.AlignmentAnchor `json:"anchors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	alignment, err := s.store.CreateAlignment(in.Name, in.BaseCaptureID, in.OtherCaptureID, in.Anchors)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, alignment)
}

func (s *Server) getAlignment(w http.ResponseWriter, r *http.Request) {
	versions, ok := s.store.AlignmentVersions(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown alignment")
		return
	}
	selected := versions[len(versions)-1]
	if query := r.URL.Query().Get("version"); query != "" {
		var version int
		if _, err := fmt.Sscanf(query, "%d", &version); err == nil {
			if candidate, ok := s.store.GetAlignmentVersion(r.PathValue("id"), version); ok {
				selected = candidate
			}
		}
	}
	writeJSON(w, map[string]any{
		"alignment": selected,
		"versions":  versions,
		"estimate":  align.EstimateOffsets(selected.Anchors),
	})
}

func (s *Server) addAlignmentVersion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Anchors []model.AlignmentAnchor `json:"anchors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	alignment, err := s.store.AddAlignmentVersion(r.PathValue("id"), in.Anchors)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"alignment": alignment,
		"estimate":  align.EstimateOffsets(alignment.Anchors),
	})
}

// alignmentPair 解析请求中的两个结果并校验它们分别属于对齐组的基准/对照采样，
// 保证跨采样比较不会混用不属于该对齐关系的配置或结果。
func (s *Server) alignmentPair(w http.ResponseWriter, r *http.Request) (model.Alignment, model.Result, model.Result, model.Config, model.Config, bool) {
	var in struct {
		Version       int    `json:"version"`
		BaseResultID  string `json:"baseResultId"`
		OtherResultID string `json:"otherResultId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return model.Alignment{}, model.Result{}, model.Result{}, model.Config{}, model.Config{}, false
	}
	alignment, ok := s.store.GetAlignmentVersion(r.PathValue("id"), in.Version)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown alignment or version")
		return model.Alignment{}, model.Result{}, model.Result{}, model.Config{}, model.Config{}, false
	}
	base, ok := s.store.GetResult(in.BaseResultID)
	if !ok || base.CaptureID != alignment.BaseCaptureID {
		writeError(w, http.StatusBadRequest, "基准结果不属于该对齐组的基准采样")
		return model.Alignment{}, model.Result{}, model.Result{}, model.Config{}, model.Config{}, false
	}
	other, ok := s.store.GetResult(in.OtherResultID)
	if !ok || other.CaptureID != alignment.OtherCaptureID {
		writeError(w, http.StatusBadRequest, "对照结果不属于该对齐组的对照采样")
		return model.Alignment{}, model.Result{}, model.Result{}, model.Config{}, model.Config{}, false
	}
	baseCfg, ok := s.store.GetConfig(base.ConfigID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown base config")
		return model.Alignment{}, model.Result{}, model.Result{}, model.Config{}, model.Config{}, false
	}
	otherCfg, ok := s.store.GetConfig(other.ConfigID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown other config")
		return model.Alignment{}, model.Result{}, model.Result{}, model.Config{}, model.Config{}, false
	}
	return alignment, base, other, baseCfg, otherCfg, true
}

func (s *Server) alignmentCandidates(w http.ResponseWriter, r *http.Request) {
	_, base, other, _, _, ok := s.alignmentPair(w, r)
	if !ok {
		return
	}
	accepted, excluded := align.AnchorCandidates(base.Markers, other.Markers)
	writeJSON(w, map[string]any{"accepted": accepted, "excluded": excluded})
}

func (s *Server) compareAlignment(w http.ResponseWriter, r *http.Request) {
	alignment, base, other, baseCfg, otherCfg, ok := s.alignmentPair(w, r)
	if !ok {
		return
	}
	writeJSON(w, align.CompareResults(alignment, base, other, baseCfg, otherCfg))
}

func (s *Server) diffResults(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BaseResultID  string `json:"baseResultId"`
		OtherResultID string `json:"otherResultId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	base, ok := s.store.GetResult(in.BaseResultID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown base result")
		return
	}
	other, ok := s.store.GetResult(in.OtherResultID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown other result")
		return
	}
	if base.CaptureID != other.CaptureID {
		writeError(w, http.StatusBadRequest, "只能对照同一采样的两个解码版本")
		return
	}
	baseCfg, ok := s.store.GetConfig(base.ConfigID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown base config")
		return
	}
	otherCfg, ok := s.store.GetConfig(other.ConfigID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown other config")
		return
	}
	writeJSON(w, compare.Results(base, other, baseCfg, otherCfg))
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
