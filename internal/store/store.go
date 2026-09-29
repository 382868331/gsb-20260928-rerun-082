// Package store keeps the three versioned artifacts apart:
// sample configs, decode configs and decode results. Nothing is mutated in
// place, so changing a decode config and recomputing leaves every previous
// result available for comparison.
package store

import (
	"fmt"
	"sort"
	"sync"

	"logicstation/internal/model"
)

type Store struct {
	mu sync.RWMutex

	sampleCfgs map[string]model.SampleConfig
	decodeCfgs map[string]model.DecodeConfig
	results    map[string]model.DecodeResult

	// blocks holds appended sample blocks per sample-config version, in order.
	blocks map[string][]model.SampleBlock
}

func New() *Store {
	return &Store{
		sampleCfgs: map[string]model.SampleConfig{},
		decodeCfgs: map[string]model.DecodeConfig{},
		results:    map[string]model.DecodeResult{},
		blocks:     map[string][]model.SampleBlock{},
	}
}

func (s *Store) PutSampleConfig(cfg model.SampleConfig) error {
	if cfg.Version == "" {
		return fmt.Errorf("store: sample config version is required")
	}
	if cfg.PeriodNs <= 0 {
		return fmt.Errorf("store: periodNs must be positive")
	}
	if len(cfg.Channels) == 0 {
		return fmt.Errorf("store: at least one channel required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sampleCfgs[cfg.Version]; exists {
		return fmt.Errorf("store: sample config %q already exists (versions are immutable)", cfg.Version)
	}
	s.sampleCfgs[cfg.Version] = cfg
	s.blocks[cfg.Version] = nil
	return nil
}

func (s *Store) GetSampleConfig(v string) (model.SampleConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.sampleCfgs[v]
	return cfg, ok
}

func (s *Store) PutDecodeConfig(cfg model.DecodeConfig) error {
	if cfg.Version == "" {
		return fmt.Errorf("store: decode config version is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.decodeCfgs[cfg.Version]; exists {
		return fmt.Errorf("store: decode config %q already exists (versions are immutable)", cfg.Version)
	}
	s.decodeCfgs[cfg.Version] = cfg
	return nil
}

func (s *Store) GetDecodeConfig(v string) (model.DecodeConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.decodeCfgs[v]
	return cfg, ok
}

// AppendBlock adds a chunk of samples. Blocks must be ordered and must not
// overlap; a jump in StartIndex encodes a capture gap.
func (s *Store) AppendBlock(block model.SampleBlock) error {
	if block.Count != len(block.Levels) {
		return fmt.Errorf("store: block count %d != levels length %d", block.Count, len(block.Levels))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sampleCfgs[block.ConfigVersion]; !ok {
		return fmt.Errorf("store: unknown sample config %q", block.ConfigVersion)
	}
	blocks := s.blocks[block.ConfigVersion]
	if len(blocks) > 0 {
		prev := blocks[len(blocks)-1]
		want := prev.StartIndex + int64(prev.Count)
		if block.StartIndex < want {
			return fmt.Errorf("store: block starting %d overlaps previous end %d", block.StartIndex, want)
		}
	}
	s.blocks[block.ConfigVersion] = append(blocks, block)
	return nil
}

func (s *Store) Blocks(sampleCfgVer string) []model.SampleBlock {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.SampleBlock, len(s.blocks[sampleCfgVer]))
	copy(out, s.blocks[sampleCfgVer])
	return out
}

func (s *Store) PutResult(res model.DecodeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[res.Version] = res
}

func (s *Store) GetResult(v string) (model.DecodeResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.results[v]
	return r, ok
}

// ResultVersions returns all result versions sorted by version string.
func (s *Store) ResultVersions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vs := make([]string, 0, len(s.results))
	for v := range s.results {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
}

func (s *Store) SampleConfigVersions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vs := make([]string, 0, len(s.sampleCfgs))
	for v := range s.sampleCfgs {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
}

func (s *Store) DecodeConfigVersions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vs := make([]string, 0, len(s.decodeCfgs))
	for v := range s.decodeCfgs {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
}
