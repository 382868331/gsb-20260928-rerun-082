// Package decode holds protocol decoders. Decoders are streaming: sample
// blocks are fed in order and partial frames carry over block boundaries, so a
// message split across blocks decodes exactly as if it arrived in one block.
package decode

import (
	"fmt"
	"math"

	"logicstation/internal/model"
)

// Decoder turns level samples into protocol-specific items.
// New protocols register here later without touching capture/version storage.
type Decoder interface {
	FeedBlock(block model.SampleBlock) []model.Item
	NotifyGap(startNs, endNs int64) []model.Item
	Flush() []model.Item
}

// New builds the decoder for a decode config. Unknown protocols fail loudly
// rather than silently producing empty results.
func New(cfg model.DecodeConfig, sampleCfg model.SampleConfig) (Decoder, error) {
	switch cfg.Protocol {
	case "uart":
		return newUART(cfg, sampleCfg)
	default:
		return nil, fmt.Errorf("decode: unsupported protocol %q (only \"uart\" is implemented)", cfg.Protocol)
	}
}

func atoi(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// nearestInt returns the integer nearest x together with the absolute distance.
func nearestInt(x float64) (int, float64) {
	n := int(math.Round(x))
	return n, math.Abs(x - float64(n))
}
