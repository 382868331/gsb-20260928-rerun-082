// Package api exposes the station over HTTP: configs, sample ingestion,
// recompute, comparison and a built-in demo waveform.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"logicstation/internal/demo"
	"logicstation/internal/engine"
	"logicstation/internal/model"
	"logicstation/internal/store"
)

type Server struct {
	st  *store.Store
	eng *engine.Engine
	mux *http.ServeMux
}

func NewServer(st *store.Store, eng *engine.Engine) *Server {
	s := &Server{st: st, eng: eng, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("GET /api/config/sample", s.listSampleConfigs)
	s.mux.HandleFunc("PUT /api/config/sample", s.putSampleConfig)
	s.mux.HandleFunc("GET /api/config/decode", s.listDecodeConfigs)
	s.mux.HandleFunc("PUT /api/config/decode", s.putDecodeConfig)
	s.mux.HandleFunc("GET /api/blocks", s.getBlocks)
	s.mux.HandleFunc("POST /api/blocks", s.appendBlock)
	s.mux.HandleFunc("POST /api/decode", s.recompute)
	s.mux.HandleFunc("GET /api/results", s.listResults)
	s.mux.HandleFunc("GET /api/result", s.getResult)
	s.mux.HandleFunc("POST /api/compare", s.compare)
	s.mux.HandleFunc("POST /api/demo", s.loadDemo)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func (s *Server) listSampleConfigs(w http.ResponseWriter, r *http.Request) {
	vers := s.st.SampleConfigVersions()
	out := make([]model.SampleConfig, 0, len(vers))
	for _, v := range vers {
		cfg, _ := s.st.GetSampleConfig(v)
		out = append(out, cfg)
	}
	writeJSON(w, 200, out)
}

func (s *Server) putSampleConfig(w http.ResponseWriter, r *http.Request) {
	var cfg model.SampleConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.st.PutSampleConfig(cfg); err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 201, cfg)
}

func (s *Server) listDecodeConfigs(w http.ResponseWriter, r *http.Request) {
	vers := s.st.DecodeConfigVersions()
	out := make([]model.DecodeConfig, 0, len(vers))
	for _, v := range vers {
		cfg, _ := s.st.GetDecodeConfig(v)
		out = append(out, cfg)
	}
	writeJSON(w, 200, out)
}

func (s *Server) putDecodeConfig(w http.ResponseWriter, r *http.Request) {
	var cfg model.DecodeConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.st.PutDecodeConfig(cfg); err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 201, cfg)
}

func (s *Server) getBlocks(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query().Get("sampleConfig")
	if v == "" {
		writeErr(w, 400, errMissing("sampleConfig"))
		return
	}
	writeJSON(w, 200, s.st.Blocks(v))
}

func (s *Server) appendBlock(w http.ResponseWriter, r *http.Request) {
	var block model.SampleBlock
	if err := json.NewDecoder(r.Body).Decode(&block); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.st.AppendBlock(block); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, block)
}

type decodeReq struct {
	SampleConfig string `json:"sampleConfig"`
	DecodeConfig string `json:"decodeConfig"`
}

func (s *Server) recompute(w http.ResponseWriter, r *http.Request) {
	var req decodeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := s.eng.Recompute(req.SampleConfig, req.DecodeConfig)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) listResults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.st.ResultVersions())
}

func (s *Server) getResult(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query().Get("version")
	res, ok := s.st.GetResult(v)
	if !ok {
		writeErr(w, 404, errMissing("result "+v))
		return
	}
	writeJSON(w, 200, res)
}

type compareReq struct {
	A string `json:"a"`
	B string `json:"b"`
}

func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	var req compareReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	diff, err := s.eng.Compare(req.A, req.B)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, diff)
}

// demoReq provisions a fresh sample config, generates a known waveform and
// appends its blocks in one call, so the UI immediately has waveform + ground
// truth to check the decoder against.
type demoReq struct {
	PeriodNs    int64    `json:"periodNs"`
	Channels    []string `json:"channels"`
	Channel     string   `json:"channel"`
	BaudRate    int      `json:"baudRate"`
	DataBits    int      `json:"dataBits"`
	Parity      string   `json:"parity"`
	StopBits    float64  `json:"stopBits"`
	Invert      bool     `json:"invert"`
	Text        string   `json:"text"`
	GapAfter    int      `json:"gapAfter"`
	GapSkip     int      `json:"gapSkip"`
	GlitchFrame int      `json:"glitchFrame"`
	GlitchBit   int      `json:"glitchBit"`
	GlitchBy    float64  `json:"glitchBySamples"`
}

type demoResp struct {
	SampleConfig model.SampleConfig  `json:"sampleConfig"`
	Blocks       []model.SampleBlock `json:"blocks"`
}

func (s *Server) loadDemo(w http.ResponseWriter, r *http.Request) {
	var req demoReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if req.PeriodNs <= 0 {
		req.PeriodNs = 1000 // 1 MHz sampling
	}
	if len(req.Channels) == 0 {
		req.Channels = []string{"clk", "tx", "rx"}
	}
	if req.Channel == "" {
		req.Channel = "tx"
	}
	if req.BaudRate == 0 {
		req.BaudRate = 9600
	}
	ver := "smp-demo-"
	for i := 1; ; i++ {
		candidate := ver + strconv.Itoa(i)
		if _, ok := s.st.GetSampleConfig(candidate); !ok {
			ver = candidate
			break
		}
	}
	cfg := model.SampleConfig{Version: ver, Channels: req.Channels, PeriodNs: req.PeriodNs,
		CreatedAt: nowRFC(), Comment: "built-in demo waveform"}
	if err := s.st.PutSampleConfig(cfg); err != nil {
		writeErr(w, 400, err)
		return
	}
	blocks := demo.Generate(cfg, demo.Params{
		Channel: req.Channel, BaudRate: req.BaudRate, DataBits: req.DataBits,
		Parity: req.Parity, StopBits: req.StopBits, Invert: req.Invert,
		Bytes: []byte(req.Text), GapAfter: req.GapAfter, GapSkip: req.GapSkip,
		GlitchFrame: req.GlitchFrame, GlitchBit: req.GlitchBit, GlitchSamples: req.GlitchBy,
	})
	for _, b := range blocks {
		if err := s.st.AppendBlock(b); err != nil {
			writeErr(w, 400, err)
			return
		}
	}
	writeJSON(w, 201, demoResp{SampleConfig: cfg, Blocks: blocks})
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }

func errMissing(what string) error { return simpleErr("missing " + what) }

func nowRFC() string { return time.Now().UTC().Format(time.RFC3339Nano) }
