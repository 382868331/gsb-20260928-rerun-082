package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"logicstation/internal/decode"
	"logicstation/internal/model"
)

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type Store struct {
	mu         sync.RWMutex
	dataPath   string
	captures   map[string]model.Capture
	chunks     map[string][]model.Chunk
	configs    map[string][]model.Config
	results    map[string]model.Result
	alignments map[string]model.Alignment
}

func New() *Store {
	return &Store{
		captures:   map[string]model.Capture{},
		chunks:     map[string][]model.Chunk{},
		configs:    map[string][]model.Config{},
		results:    map[string]model.Result{},
		alignments: map[string]model.Alignment{},
	}
}

type snapshot struct {
	Captures   []model.Capture           `json:"captures"`
	Chunks     map[string][]model.Chunk  `json:"chunks"`
	Configs    map[string][]model.Config `json:"configs"`
	Results    []model.Result            `json:"results"`
	Alignments []model.Alignment         `json:"alignments"`
}

func NewPersistent(path string) (*Store, error) {
	s := New()
	s.dataPath = path
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var data snapshot
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("load alignment data: %w", err)
	}
	for _, c := range data.Captures {
		s.captures[c.ID] = c
	}
	if data.Chunks != nil {
		s.chunks = data.Chunks
	}
	if data.Configs != nil {
		s.configs = data.Configs
	}
	for _, c := range data.Captures {
		if _, ok := s.chunks[c.ID]; !ok {
			s.chunks[c.ID] = []model.Chunk{}
		}
		if _, ok := s.configs[c.ID]; !ok {
			s.configs[c.ID] = []model.Config{}
		}
	}
	for _, r := range data.Results {
		s.results[r.ID] = r
	}
	for _, a := range data.Alignments {
		s.alignments[a.ID] = a
	}
	return s, nil
}

func (s *Store) saveLocked() {
	if s.dataPath == "" {
		return
	}
	data := snapshot{Chunks: s.chunks, Configs: s.configs}
	for _, c := range s.captures {
		data.Captures = append(data.Captures, c)
	}
	for _, r := range s.results {
		data.Results = append(data.Results, r)
	}
	for _, a := range s.alignments {
		data.Alignments = append(data.Alignments, a)
	}
	sort.Slice(data.Captures, func(i, j int) bool { return data.Captures[i].CreatedAt.Before(data.Captures[j].CreatedAt) })
	payload, _ := json.MarshalIndent(data, "", "  ")
	if err := os.MkdirAll(filepath.Dir(s.dataPath), 0o755); err == nil {
		_ = os.WriteFile(s.dataPath, payload, 0o600)
	}
}

func supportsProtocol(protocol string) bool {
	for _, supported := range decode.SupportedProtocols() {
		if protocol == supported {
			return true
		}
	}
	return false
}

func (s *Store) CreateCapture(name string, channels []string) model.Capture {
	s.mu.Lock()
	defer s.mu.Unlock()
	capture := model.Capture{ID: newID(), Name: name, ChannelNames: channels, CreatedAt: time.Now().UTC()}
	s.captures[capture.ID] = capture
	s.chunks[capture.ID] = []model.Chunk{}
	s.configs[capture.ID] = []model.Config{}
	s.saveLocked()
	return capture
}

func (s *Store) AddChunks(in []model.Chunk) ([]model.Chunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Chunk, 0, len(in))
	for _, chunk := range in {
		if _, ok := s.captures[chunk.CaptureID]; !ok {
			return nil, fmt.Errorf("unknown capture %q", chunk.CaptureID)
		}
		if chunk.IntervalNs <= 0 {
			return nil, fmt.Errorf("intervalNs must be positive")
		}
		if chunk.ID == "" {
			chunk.ID = newID()
		}
		s.chunks[chunk.CaptureID] = append(s.chunks[chunk.CaptureID], chunk)
		sort.SliceStable(s.chunks[chunk.CaptureID], func(i, j int) bool {
			return s.chunks[chunk.CaptureID][i].Index < s.chunks[chunk.CaptureID][j].Index
		})
		out = append(out, chunk)
	}
	s.saveLocked()
	return out, nil
}

func (s *Store) ListCaptures() []model.Capture {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Capture, 0, len(s.captures))
	for _, capture := range s.captures {
		out = append(out, capture)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// FindCaptureByName 按名称返回既有采样，保证重复载入示例时复用同一份采样身份，
// 而不是再建一个同名新示例（用户保存的配置/结果对照因此可以重开）。
func (s *Store) FindCaptureByName(name string) (model.Capture, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, capture := range s.captures {
		if capture.Name == name {
			return capture, true
		}
	}
	return model.Capture{}, false
}

func (s *Store) GetCapture(id string) (model.Capture, []model.Chunk, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	capture, ok := s.captures[id]
	return capture, append([]model.Chunk(nil), s.chunks[id]...), ok
}

func (s *Store) CreateConfig(cfg model.Config) (model.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.captures[cfg.CaptureID]; !ok {
		return model.Config{}, fmt.Errorf("unknown capture %q", cfg.CaptureID)
	}
	if !supportsProtocol(cfg.Protocol) {
		return model.Config{}, fmt.Errorf("supported protocols are: %v", decode.SupportedProtocols())
	}
	cfg.Version = len(s.configs[cfg.CaptureID]) + 1
	if cfg.ID == "" {
		cfg.ID = newID()
	}
	if cfg.Name == "" {
		cfg.Name = fmt.Sprintf("%s 配置 v%d", cfg.Protocol, cfg.Version)
	}
	cfg.CreatedAt = time.Now().UTC()
	s.configs[cfg.CaptureID] = append(s.configs[cfg.CaptureID], cfg)
	s.saveLocked()
	return cfg, nil
}

func (s *Store) ListConfigs(captureID string) []model.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Config(nil), s.configs[captureID]...)
}

func (s *Store) GetConfig(id string) (model.Config, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, configs := range s.configs {
		for _, cfg := range configs {
			if cfg.ID == id {
				return cfg, true
			}
		}
	}
	return model.Config{}, false
}

func (s *Store) Recalculate(configID string) (model.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cfg model.Config
	var chunks []model.Chunk
	var capture model.Capture
	found := false
	for captureID, configs := range s.configs {
		_ = captureID
		for _, candidate := range configs {
			if candidate.ID == configID {
				cfg, capture, found = candidate, s.captures[candidate.CaptureID], true
				chunks = append([]model.Chunk(nil), s.chunks[candidate.CaptureID]...)
			}
		}
	}
	if !found {
		return model.Result{}, fmt.Errorf("unknown config %q", configID)
	}
	result, err := decode.Decode(capture.Name, chunks, cfg)
	if err != nil {
		return model.Result{}, err
	}
	result.ID = fmt.Sprintf("%s:v%d-result", cfg.ID, cfg.Version)
	s.results[result.ID] = result
	s.saveLocked()
	return result, nil
}

func (s *Store) ListResults(captureID string) []model.Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Result
	for _, result := range s.results {
		if result.CaptureID == captureID {
			out = append(out, result)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ConfigVersion < out[j].ConfigVersion })
	return out
}

func (s *Store) GetResult(id string) (model.Result, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result, ok := s.results[id]
	return result, ok
}

func (s *Store) SaveAlignment(in model.Alignment) (model.Alignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.ID != "" {
		if _, exists := s.alignments[in.ID]; exists {
			return model.Alignment{}, fmt.Errorf("alignment %q already exists; changing anchors creates a new comparison", in.ID)
		}
	}
	if _, ok := s.captures[in.BaseCaptureID]; !ok {
		return model.Alignment{}, fmt.Errorf("unknown base capture %q", in.BaseCaptureID)
	}
	if _, ok := s.captures[in.OtherCaptureID]; !ok {
		return model.Alignment{}, fmt.Errorf("unknown other capture %q", in.OtherCaptureID)
	}
	if in.BaseCaptureID == in.OtherCaptureID {
		return model.Alignment{}, fmt.Errorf("cross-capture alignment requires different captures")
	}
	if in.ID == "" {
		in.ID = newID()
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now().UTC()
	}
	s.alignments[in.ID] = in
	s.saveLocked()
	return in, nil
}

func (s *Store) GetAlignment(id string) (model.Alignment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.alignments[id]
	return a, ok
}

func (s *Store) ListAlignments() []model.Alignment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Alignment, 0, len(s.alignments))
	for _, a := range s.alignments {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
