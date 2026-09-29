// Package demo builds known UART level waveforms so the UI can verify decoded
// content and start/end instants against the waveform itself. It can inject
// the two capture-side fault classes: a sample gap and a transmitter clock
// anomaly (one bit cell stretched so an edge lands off-grid).
package demo

import (
	"fmt"
	"math"

	"logicstation/internal/model"
)

type Params struct {
	Channel  string
	BaudRate int
	DataBits int
	Parity   string // none|even|odd
	StopBits float64
	Invert   bool
	Bytes    []byte

	// GapAfter splits samples into two blocks after this many samples and
	// skips GapSkip samples between them (0 disables).
	GapAfter int
	GapSkip  int

	// GlitchFrame >= 0 stretches cell GlitchBit (0=start) of that frame by
	// GlitchSamples sample periods.
	GlitchFrame   int
	GlitchBit     int
	GlitchSamples float64
}

// Generate rasterizes UART frames into sample blocks under cfg.
func Generate(cfg model.SampleConfig, p Params) []model.SampleBlock {
	if p.DataBits == 0 {
		p.DataBits = 8
	}
	if p.Parity == "" {
		p.Parity = "none"
	}
	if p.StopBits == 0 {
		p.StopBits = 1
	}
	chanIdx := 0
	for i, name := range cfg.Channels {
		if name == p.Channel {
			chanIdx = i
		}
	}
	bitNs := 1e9 / float64(p.BaudRate)
	period := float64(cfg.PeriodNs)

	type cell struct {
		name string
		lvl  int
		dur  float64 // ns
	}
	type edge struct {
		t   float64
		lvl int
	}
	edges := []edge{{0, 1}}
	t := 4 * bitNs // idle lead-in

	for fi, b := range p.Bytes {
		cells := []cell{{"start", 0, bitNs}}
		for i := 0; i < p.DataBits; i++ {
			cells = append(cells, cell{fmt.Sprintf("d%d", i), int((b >> uint(i)) & 1), bitNs})
		}
		if p.Parity != "none" {
			ones := 0
			for i := 0; i < p.DataBits; i++ {
				ones += int((b >> uint(i)) & 1)
			}
			pv := ones % 2
			if p.Parity == "odd" {
				pv = (ones + 1) % 2
			}
			cells = append(cells, cell{"parity", pv, bitNs})
		}
		stopCells := 1
		if p.StopBits >= 1.4 {
			stopCells = 2
		}
		for i := 0; i < stopCells; i++ {
			cells = append(cells, cell{"stop", 1, bitNs})
		}
		if p.GlitchFrame == fi {
			if p.GlitchBit >= 0 && p.GlitchBit < len(cells) {
				cells[p.GlitchBit].dur += p.GlitchSamples * period
			}
		}
		for _, c := range cells {
			edges = append(edges, edge{t, c.lvl})
			t += c.dur
		}
		t += bitNs // idle gap before next frame
	}
	t += 3 * bitNs // idle tail

	lvlAt := func(idx int) int {
		now := float64(idx)*period + period/2
		lvl := 1
		for _, e := range edges {
			if e.t <= now+1e-6 {
				lvl = e.lvl
			}
		}
		if p.Invert {
			lvl = 1 - lvl
		}
		return lvl
	}
	nSamples := int(math.Ceil(t/period)) + 1
	pack := func(start, count int) model.SampleBlock {
		lv := make([]byte, count)
		for i := 0; i < count; i++ {
			if lvlAt(start+i) == 1 {
				lv[i] = 1 << uint(chanIdx)
			}
		}
		return model.SampleBlock{ConfigVersion: cfg.Version, StartIndex: int64(start), Count: count, Levels: lv}
	}

	if p.GapAfter <= 0 || p.GapAfter >= nSamples || p.GapSkip <= 0 {
		return []model.SampleBlock{pack(0, nSamples)}
	}
	b1 := pack(0, p.GapAfter)
	b2 := pack(p.GapAfter+p.GapSkip, nSamples-p.GapAfter)
	return []model.SampleBlock{b1, b2}
}
