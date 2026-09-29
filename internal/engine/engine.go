// Package engine runs decoders over stored sample blocks. It is a pure
// recompute layer: given (sample config version, decode config version) it
// always produces the same result, keyed by its own result version.
package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"logicstation/internal/decode"
	"logicstation/internal/model"
	"logicstation/internal/store"
)

type Engine struct{ st *store.Store }

func New(st *store.Store) *Engine { return &Engine{st: st} }

// ResultVersion derives a stable version from the two config versions.
// Re-running the same pair yields the same version (and identical content),
// while a new decode config produces a distinct, comparable result.
func ResultVersion(sampleCfgVer, decodeCfgVer string) string {
	h := sha256.Sum256([]byte("v1:" + sampleCfgVer + ":" + decodeCfgVer))
	return "res-" + hex.EncodeToString(h[:10])
}

// Recompute decodes every stored block for the given config pair. Blocks are
// fed in order; whenever a block starts after the previous block ended, the
// decoder is notified of the gap first so a frame straddling it surfaces as a
// sample_gap error instead of a stitched-together byte.
func (e *Engine) Recompute(sampleCfgVer, decodeCfgVer string) (model.DecodeResult, error) {
	sampleCfg, ok := e.st.GetSampleConfig(sampleCfgVer)
	if !ok {
		return model.DecodeResult{}, fmt.Errorf("engine: unknown sample config %q", sampleCfgVer)
	}
	decodeCfg, ok := e.st.GetDecodeConfig(decodeCfgVer)
	if !ok {
		return model.DecodeResult{}, fmt.Errorf("engine: unknown decode config %q", decodeCfgVer)
	}
	dec, err := decode.New(decodeCfg, sampleCfg)
	if err != nil {
		return model.DecodeResult{}, err
	}

	blocks := e.st.Blocks(sampleCfgVer)
	res := model.DecodeResult{
		Version:         ResultVersion(sampleCfgVer, decodeCfgVer),
		SampleConfigVer: sampleCfgVer,
		DecodeConfigVer: decodeCfgVer,
		Protocol:        decodeCfg.Protocol,
		Items:           []model.Item{},
		Bytes:           []int{},
		ComputedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}

	var nextExpected int64
	for i, block := range blocks {
		if i > 0 && block.StartIndex > nextExpected {
			gapStart := nextExpected * sampleCfg.PeriodNs
			gapEnd := block.StartIndex * sampleCfg.PeriodNs
			res.Items = append(res.Items, dec.NotifyGap(gapStart, gapEnd)...)
		}
		res.Items = append(res.Items, dec.FeedBlock(block)...)
		nextExpected = block.StartIndex + int64(block.Count)
	}
	res.Items = append(res.Items, dec.Flush()...)

	for _, item := range res.Items {
		switch item.Kind {
		case model.KindData:
			res.Bytes = append(res.Bytes, *item.Value)
		case model.KindError:
			res.ErrorCount++
		}
	}

	e.st.PutResult(res)
	return res, nil
}

// Compare reports how two result versions differ, aligned by start time.
// It never errors on content differences: differences are the point.
type Diff struct {
	A           string        `json:"a"`
	B           string        `json:"b"`
	Same        bool          `json:"same"`
	OnlyInA     []model.Item  `json:"onlyInA"`
	OnlyInB     []model.Item  `json:"onlyInB"`
	Changed     []ChangedItem `json:"changed"`
	ByteStreamA []int         `json:"byteStreamA"`
	ByteStreamB []int         `json:"byteStreamB"`
}

type ChangedItem struct {
	StartNs int64       `json:"startNs"`
	A       *model.Item `json:"a"`
	B       *model.Item `json:"b"`
}

func (e *Engine) Compare(verA, verB string) (Diff, error) {
	ra, ok := e.st.GetResult(verA)
	if !ok {
		return Diff{}, fmt.Errorf("engine: unknown result %q", verA)
	}
	rb, ok := e.st.GetResult(verB)
	if !ok {
		return Diff{}, fmt.Errorf("engine: unknown result %q", verB)
	}
	diff := Diff{A: verA, B: verB, ByteStreamA: ra.Bytes, ByteStreamB: rb.Bytes}

	key := func(it model.Item) string {
		return fmt.Sprintf("%d|%s", it.StartNs, it.Channel)
	}
	ma := map[string]model.Item{}
	mb := map[string]model.Item{}
	for _, it := range ra.Items {
		ma[key(it)] = it
	}
	for _, it := range rb.Items {
		mb[key(it)] = it
	}
	for k, va := range ma {
		vb, ok := mb[k]
		if !ok {
			diff.OnlyInA = append(diff.OnlyInA, va)
			continue
		}
		if !itemsEqual(va, vb) {
			va2, vb2 := va, vb
			diff.Changed = append(diff.Changed, ChangedItem{StartNs: va.StartNs, A: &va2, B: &vb2})
		}
	}
	for k, vb := range mb {
		if _, ok := ma[k]; !ok {
			diff.OnlyInB = append(diff.OnlyInB, vb)
		}
	}
	diff.Same = len(diff.OnlyInA) == 0 && len(diff.OnlyInB) == 0 && len(diff.Changed) == 0 &&
		fmt.Sprint(ra.Bytes) == fmt.Sprint(rb.Bytes)
	return diff, nil
}

func itemsEqual(a, b model.Item) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
