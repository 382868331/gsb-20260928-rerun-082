package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"logicstation/internal/alignment"
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
	s.mux.HandleFunc("GET /api/alignments", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.store.ListAlignments()) })
	s.mux.HandleFunc("GET /api/alignments/{id}", s.getAlignment)
	s.mux.HandleFunc("POST /api/alignments", s.createAlignment)
	s.mux.HandleFunc("POST /api/results/cross-diff", s.crossDiff)
	if staticDir != "" {
		s.mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	}
}

type anchorRequest struct {
	BaseMarkerID         string `json:"baseMarkerId"`
	OtherMarkerID        string `json:"otherMarkerId"`
	BaseSegmentID        string `json:"baseSegmentId"`
	OtherSegmentID       string `json:"otherSegmentId"`
	UncertaintyNs        int64  `json:"uncertaintyNs"`
	ManualOrderConfirmed bool   `json:"manualOrderConfirmed"`
	Evidence             string `json:"evidence"`
}

func (s *Server) createAlignment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name          string          `json:"name"`
		BaseResultID  string          `json:"baseResultId"`
		OtherResultID string          `json:"otherResultId"`
		Anchors       []anchorRequest `json:"anchors"`
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
	if base.CaptureID == other.CaptureID {
		writeError(w, http.StatusBadRequest, "跨采样对齐需要两个不同采样")
		return
	}
	anchors := []model.AlignmentAnchor{}
	for i, req := range in.Anchors {
		bm, ok1 := findMarker(base.Markers, req.BaseMarkerID)
		om, ok2 := findMarker(other.Markers, req.OtherMarkerID)
		if !ok1 || !ok2 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("anchor %d marker not found", i+1))
			return
		}
		baseSegmentID := req.BaseSegmentID
		if baseSegmentID == "" {
			baseSegmentID = bm.SegmentID
		}
		otherSegmentID := req.OtherSegmentID
		if otherSegmentID == "" {
			otherSegmentID = om.SegmentID
		}
		anchors = append(anchors, model.AlignmentAnchor{ID: fmt.Sprintf("anchor-%d", i+1), Sequence: i + 1, BaseMarkerID: bm.ID, OtherMarkerID: om.ID, BaseSegmentID: baseSegmentID, OtherSegmentID: otherSegmentID, BaseChunkID: bm.ChunkID, OtherChunkID: om.ChunkID, BaseTimeNs: bm.StartNs, OtherTimeNs: om.StartNs, UncertaintyNs: req.UncertaintyNs, ManualOrderConfirmed: req.ManualOrderConfirmed, Evidence: req.Evidence})
	}
	_, baseChunks, _ := s.store.GetCapture(base.CaptureID)
	_, otherChunks, _ := s.store.GetCapture(other.CaptureID)
	align, mapping, err := alignment.Build(model.Alignment{Name: in.Name, BaseCaptureID: base.CaptureID, OtherCaptureID: other.CaptureID, BaseResultID: base.ID, OtherResultID: other.ID, Anchors: anchors}, baseChunks, otherChunks)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	align.ExcludedCandidates = buildCandidates(align, mapping, base.Markers, other.Markers)
	saved, err := s.store.SaveAlignment(align)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, map[string]any{"alignment": saved, "segments": mapping.Segments, "status": saved.Status})
}

func buildCandidates(align model.Alignment, mapping alignment.Mapping, baseMarkers, otherMarkers []model.Marker) []model.AlignmentCandidate {
	out := []model.AlignmentCandidate{}
	for _, a := range align.Anchors {
		out = append(out, model.AlignmentCandidate{ID: fmt.Sprintf("%s-accepted", a.ID), AnchorID: a.ID, BaseMarkerID: a.BaseMarkerID, OtherMarkerID: a.OtherMarkerID, BaseSegmentID: a.BaseSegmentID, OtherSegmentID: a.OtherSegmentID, BaseTimeNs: a.BaseTimeNs, OtherTimeNs: a.OtherTimeNs, Score: 100, Status: alignment.CandidateAccepted, Reason: "用户确认的共同事件"})
		bm, _ := findMarker(baseMarkers, a.BaseMarkerID)
		for _, om := range otherMarkers {
			if om.ID == a.OtherMarkerID || !sameSignature(&bm, &om) {
				continue
			}
			if om.SegmentID != a.OtherSegmentID {
				out = append(out, model.AlignmentCandidate{ID: fmt.Sprintf("%s-excluded-%d", a.ID, len(out)), AnchorID: a.ID, BaseMarkerID: bm.ID, OtherMarkerID: om.ID, BaseSegmentID: bm.SegmentID, OtherSegmentID: om.SegmentID, BaseTimeNs: bm.StartNs, OtherTimeNs: om.StartNs, Score: 0, Status: alignment.CandidateExcluded, Reason: "另一重置轮次的同特征事件：段身份不同，即使本地时间同名也不参与匹配"})
				continue
			}
			iv, _ := mapping.MapInterval(a.OtherSegmentID, om.StartNs, om.EndNs, a.UncertaintyNs)
			residual := iv.Start - bm.StartNs
			abs := residual
			if abs < 0 {
				abs = -abs
			}
			score := 100 - int(abs/1000)
			if score < 0 {
				score = 0
			}
			out = append(out, model.AlignmentCandidate{ID: fmt.Sprintf("%s-excluded-%d", a.ID, len(out)), AnchorID: a.ID, BaseMarkerID: bm.ID, OtherMarkerID: om.ID, BaseSegmentID: bm.SegmentID, OtherSegmentID: om.SegmentID, BaseTimeNs: bm.StartNs, OtherTimeNs: om.StartNs, PredictedNs: iv.Start, ResidualNs: residual, ConfidenceHalf: iv.Half, Score: score, Status: alignment.CandidateExcluded, Reason: "同特征重复报文候选；仅在同一采集段及可信范围内竞争"})
		}
	}
	return out
}

func enrichLegacyAnchors(align *model.Alignment, baseMarkers, otherMarkers []model.Marker, baseChunks, otherChunks []model.Chunk) {
	model.NormalizeChunkSegments(baseChunks)
	model.NormalizeChunkSegments(otherChunks)
	backfillMarkerSegments(baseMarkers, model.CaptureSegments(align.BaseCaptureID, baseChunks))
	backfillMarkerSegments(otherMarkers, model.CaptureSegments(align.OtherCaptureID, otherChunks))
	for i := range align.Anchors {
		a := &align.Anchors[i]
		if a.Sequence == 0 {
			a.Sequence = i + 1
		}
		if bm, ok := findMarker(baseMarkers, a.BaseMarkerID); ok {
			if a.BaseSegmentID == "" {
				a.BaseSegmentID = bm.SegmentID
			}
			if a.BaseChunkID == "" {
				a.BaseChunkID = bm.ChunkID
			}
			if a.BaseTimeNs == 0 {
				a.BaseTimeNs = bm.StartNs
			}
		}
		if om, ok := findMarker(otherMarkers, a.OtherMarkerID); ok {
			if a.OtherSegmentID == "" {
				a.OtherSegmentID = om.SegmentID
			}
			if a.OtherChunkID == "" {
				a.OtherChunkID = om.ChunkID
			}
			if a.OtherTimeNs == 0 {
				a.OtherTimeNs = om.StartNs
			}
		}
	}
}

func backfillMarkerSegments(markers []model.Marker, segments []model.CaptureSegment) {
	for i := range markers {
		if markers[i].SegmentID != "" {
			continue
		}
		matches := []string{}
		for _, segment := range segments {
			if markers[i].StartNs >= segment.StartNs && markers[i].EndNs <= segment.EndNs {
				matches = append(matches, segment.ID)
			}
		}
		if len(matches) == 1 {
			markers[i].SegmentID = matches[0]
		}
	}
}

func sameSignature(a, b *model.Marker) bool {
	if a == nil || b == nil || a.Kind != b.Kind || a.Code != b.Code {
		return false
	}
	if a.Value != nil && b.Value != nil {
		return *a.Value == *b.Value
	}
	return a.Value == nil && b.Value == nil && a.RawBits == b.RawBits
}

func findMarker(markers []model.Marker, id string) (model.Marker, bool) {
	for _, m := range markers {
		if m.ID == id {
			return m, true
		}
	}
	return model.Marker{}, false
}

func (s *Server) getAlignment(w http.ResponseWriter, r *http.Request) {
	a, ok := s.store.GetAlignment(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "alignment not found")
		return
	}
	writeJSON(w, a)
}

func (s *Server) crossDiff(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AlignmentID string `json:"alignmentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	align, ok := s.store.GetAlignment(in.AlignmentID)
	if !ok {
		writeError(w, http.StatusNotFound, "alignment not found")
		return
	}
	base, ok := s.store.GetResult(align.BaseResultID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown base result")
		return
	}
	other, ok := s.store.GetResult(align.OtherResultID)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown other result")
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
	_, baseChunks, _ := s.store.GetCapture(align.BaseCaptureID)
	_, otherChunks, _ := s.store.GetCapture(align.OtherCaptureID)
	base.Markers = append([]model.Marker(nil), base.Markers...)
	other.Markers = append([]model.Marker(nil), other.Markers...)
	enrichLegacyAnchors(&align, base.Markers, other.Markers, baseChunks, otherChunks)
	align, mapping, err := alignment.Build(align, baseChunks, otherChunks)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, compare.CrossResults(align, mapping, base, other, baseCfg, otherCfg, baseChunks, otherChunks))
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
