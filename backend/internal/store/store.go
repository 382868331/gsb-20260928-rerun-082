package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	mu             sync.RWMutex
	captures       map[string]model.Capture
	chunks         map[string][]model.Chunk
	configs        map[string][]model.Config
	results        map[string]model.Result
	alignments     map[string][]model.Alignment
	alignmentOrder []string
}

func New() *Store {
	return &Store{
		captures:   map[string]model.Capture{},
		chunks:     map[string][]model.Chunk{},
		configs:    map[string][]model.Config{},
		results:    map[string]model.Result{},
		alignments: map[string][]model.Alignment{},
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

func ensureAnchorIDs(anchors []model.AlignmentAnchor) []model.AlignmentAnchor {
	out := append([]model.AlignmentAnchor(nil), anchors...)
	for i := range out {
		if out[i].ID == "" {
			out[i].ID = newID()
		}
		if out[i].Source == "" {
			out[i].Source = "manual"
		}
	}
	return out
}

// CreateAlignment 新建一组跨采样对齐依据（版本 1）。
func (s *Store) CreateAlignment(name, baseCaptureID, otherCaptureID string, anchors []model.AlignmentAnchor) (model.Alignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.captures[baseCaptureID]; !ok {
		return model.Alignment{}, fmt.Errorf("unknown capture %q", baseCaptureID)
	}
	if _, ok := s.captures[otherCaptureID]; !ok {
		return model.Alignment{}, fmt.Errorf("unknown capture %q", otherCaptureID)
	}
	groupID := newID()
	alignment := model.Alignment{
		ID:             fmt.Sprintf("%s:v1", groupID),
		GroupID:        groupID,
		Name:           name,
		BaseCaptureID:  baseCaptureID,
		OtherCaptureID: otherCaptureID,
		Version:        1,
		Anchors:        ensureAnchorIDs(anchors),
		CreatedAt:      time.Now().UTC(),
	}
	if alignment.Name == "" {
		alignment.Name = "对齐组 " + groupID[:6]
	}
	s.alignments[groupID] = []model.Alignment{alignment}
	s.alignmentOrder = append(s.alignmentOrder, groupID)
	return alignment, nil
}

// AddAlignmentVersion 修改对齐依据：生成新的不可变版本，旧版本保留，
// 已保存的解码结果与历史比较不受影响。
func (s *Store) AddAlignmentVersion(groupID string, anchors []model.AlignmentAnchor) (model.Alignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions, ok := s.alignments[groupID]
	if !ok {
		return model.Alignment{}, fmt.Errorf("unknown alignment %q", groupID)
	}
	latest := versions[len(versions)-1]
	next := model.Alignment{
		GroupID:        groupID,
		Name:           latest.Name,
		BaseCaptureID:  latest.BaseCaptureID,
		OtherCaptureID: latest.OtherCaptureID,
		Version:        latest.Version + 1,
		Anchors:        ensureAnchorIDs(anchors),
		CreatedAt:      time.Now().UTC(),
	}
	next.ID = fmt.Sprintf("%s:v%d", groupID, next.Version)
	s.alignments[groupID] = append(s.alignments[groupID], next)
	return next, nil
}

// ListAlignments 返回每组对齐的最新版本。
func (s *Store) ListAlignments() []model.Alignment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Alignment, 0, len(s.alignmentOrder))
	for _, groupID := range s.alignmentOrder {
		versions := s.alignments[groupID]
		out = append(out, versions[len(versions)-1])
	}
	return out
}

// AlignmentVersions 返回一组对齐的全部版本（含被替换的旧对齐依据）。
func (s *Store) AlignmentVersions(groupID string) ([]model.Alignment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions, ok := s.alignments[groupID]
	return append([]model.Alignment(nil), versions...), ok
}

// GetAlignmentVersion 按版本号取对齐依据；version<=0 时取最新版本。
func (s *Store) GetAlignmentVersion(groupID string, version int) (model.Alignment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions, ok := s.alignments[groupID]
	if !ok {
		return model.Alignment{}, false
	}
	if version <= 0 {
		return versions[len(versions)-1], true
	}
	for _, alignment := range versions {
		if alignment.Version == version {
			return alignment, true
		}
	}
	return model.Alignment{}, false
}
