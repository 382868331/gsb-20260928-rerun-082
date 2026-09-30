package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
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
	mu       sync.RWMutex
	path     string
	captures map[string]model.Capture
	chunks   map[string][]model.Chunk
	configs  map[string][]model.Config
	results  map[string]model.Result
}

func New(path ...string) *Store {
	s := &Store{
		captures: map[string]model.Capture{},
		chunks:   map[string][]model.Chunk{},
		configs:  map[string][]model.Config{},
		results:  map[string]model.Result{},
	}
	if len(path) > 0 {
		s.path = path[0]
		s.load()
	}
	return s
}

type snapshot struct {
	Captures map[string]model.Capture  `json:"captures"`
	Chunks   map[string][]model.Chunk  `json:"chunks"`
	Configs  map[string][]model.Config `json:"configs"`
	Results  map[string]model.Result   `json:"results"`
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return
	}
	if snap.Captures != nil {
		s.captures = snap.Captures
	}
	if snap.Chunks != nil {
		s.chunks = snap.Chunks
	}
	if snap.Configs != nil {
		s.configs = snap.Configs
	}
	if snap.Results != nil {
		s.results = snap.Results
	}
}

// persist 必须在持有写锁时调用。
func (s *Store) persist() {
	if s.path == "" {
		return
	}
	snap := snapshot{Captures: s.captures, Chunks: s.chunks, Configs: s.configs, Results: s.results}
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}

func (s *Store) CreateCapture(name string, channels []string) model.Capture {
	s.mu.Lock()
	defer s.mu.Unlock()
	capture := model.Capture{ID: newID(), Name: name, ChannelNames: channels, CreatedAt: time.Now().UTC()}
	s.captures[capture.ID] = capture
	s.chunks[capture.ID] = []model.Chunk{}
	s.configs[capture.ID] = []model.Config{}
	s.persist()
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
	s.persist()
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
	if cfg.Protocol != "uart" && cfg.Protocol != "spi" {
		return model.Config{}, fmt.Errorf("supported protocols are: %v", decode.SupportedProtocols())
	}
	cfg.Version = len(s.configs[cfg.CaptureID]) + 1
	if cfg.ID == "" {
		cfg.ID = newID()
	}
	if cfg.Name == "" {
		cfg.Name = fmt.Sprintf("%s 配置 v%d", strings.ToUpper(cfg.Protocol), cfg.Version)
	}
	cfg.CreatedAt = time.Now().UTC()
	s.configs[cfg.CaptureID] = append(s.configs[cfg.CaptureID], cfg)
	s.persist()
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
	s.persist()
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
