package decode

import (
	"testing"

	"logicstation/internal/demo"
	"logicstation/internal/model"
)

const periodNs = 1000 // 1 MHz

func baseSampleCfg() model.SampleConfig {
	return model.SampleConfig{Version: "s1", Channels: []string{"tx"}, PeriodNs: periodNs}
}

func decodeAll(t *testing.T, cfg model.DecodeConfig, sampleCfg model.SampleConfig, blocks []model.SampleBlock) []model.Item {
	t.Helper()
	dec, err := New(cfg, sampleCfg)
	if err != nil {
		t.Fatal(err)
	}
	var items []model.Item
	var next int64
	for i, b := range blocks {
		if i > 0 && b.StartIndex > next {
			items = append(items, dec.NotifyGap(next*periodNs, b.StartIndex*periodNs)...)
		}
		items = append(items, dec.FeedBlock(b)...)
		next = b.StartIndex + int64(b.Count)
	}
	items = append(items, dec.Flush()...)
	return items
}

func uartCfg() model.DecodeConfig {
	return model.DecodeConfig{Version: "d1", Protocol: "uart", Channels: []string{"tx"},
		BaudRate: 9600, DataBits: 8, Parity: "none", StopBits: 1}
}

func TestDecodeKnownBytesAndTiming(t *testing.T) {
	sampleCfg := baseSampleCfg()
	want := []byte("A7") // 0x41, 0x37
	blocks := demo.Generate(sampleCfg, demo.Params{
		Channel: "tx", BaudRate: 9600, Bytes: want,
	})
	items := decodeAll(t, uartCfg(), sampleCfg, blocks)

	var got []int
	var dataItems []model.Item
	for _, it := range items {
		if it.Kind != model.KindData {
			t.Fatalf("unexpected item: %+v", it)
		}
		got = append(got, *it.Value)
		dataItems = append(dataItems, it)
	}
	if len(got) != 2 || got[0] != 0x41 || got[1] != 0x37 {
		t.Fatalf("bytes = %v, want [65 55]", got)
	}
	bitNs := int64(1e9) / 9600
	for _, it := range dataItems {
		// 10 bit cells for 8N1
		wantDur := int64(10 * bitNs)
		if d := it.EndNs - it.StartNs; abs(d-wantDur) > periodNs {
			t.Errorf("frame duration %dns, want ~%dns", d, wantDur)
		}
		if len(it.Bits) != 10 {
			t.Errorf("bits = %d, want 10", len(it.Bits))
		}
	}
}

// A message split mid-frame across two contiguous blocks must decode the same
// as one block: half a frame carries over seamlessly.
func TestFrameSplitAcrossBlocks(t *testing.T) {
	sampleCfg := baseSampleCfg()
	blocks := demo.Generate(sampleCfg, demo.Params{
		Channel: "tx", BaudRate: 9600, Bytes: []byte("Hi"),
	})
	one := decodeAll(t, uartCfg(), sampleCfg, blocks)

	// Split the single block in the middle of the first frame.
	b := blocks[0]
	split := b.StartIndex + int64(b.Count/3) // lands inside frame 0
	b1 := model.SampleBlock{ConfigVersion: b.ConfigVersion, StartIndex: b.StartIndex,
		Count: int(split - b.StartIndex), Levels: b.Levels[:split-b.StartIndex]}
	b2 := model.SampleBlock{ConfigVersion: b.ConfigVersion, StartIndex: split,
		Count: b.Count - int(split-b.StartIndex), Levels: b.Levels[split-b.StartIndex:]}
	two := decodeAll(t, uartCfg(), sampleCfg, []model.SampleBlock{b1, b2})

	if len(two) != len(one) {
		t.Fatalf("split frames=%d, contiguous frames=%d", len(two), len(one))
	}
	for i := range one {
		if *two[i].Value != *one[i].Value || two[i].StartNs != one[i].StartNs {
			t.Errorf("item %d differs: %+v vs %+v", i, two[i], one[i])
		}
	}
}

// Two scenarios exercise capture gaps:
//  1. gap inside a frame -> sample_gap error, original byte never produced;
//  2. gap fully between frames -> no error is fabricated, decoders keep state.
func TestSampleGapHandling(t *testing.T) {
	sampleCfg := baseSampleCfg()
	b := demo.Generate(sampleCfg, demo.Params{
		Channel: "tx", BaudRate: 9600, Bytes: []byte{0x55, 0xAA},
	})[0]
	spb := int64(1e9) / 9600 / periodNs

	// Scenario 1: split 3 cells into frame 0 and skip a whole number of cells.
	frame0Start := 4 * spb
	split := frame0Start + 3*spb
	b1 := model.SampleBlock{ConfigVersion: b.ConfigVersion, StartIndex: 0,
		Count: int(split), Levels: b.Levels[:split]}
	b2 := model.SampleBlock{ConfigVersion: b.ConfigVersion, StartIndex: split + 10*spb,
		Count: b.Count - int(split), Levels: b.Levels[split:]}
	items := decodeAll(t, uartCfg(), sampleCfg, []model.SampleBlock{b1, b2})

	hasGapErr := false
	for _, it := range items {
		if it.Kind == model.KindError && it.ErrCode == model.ErrSampleGap {
			hasGapErr = true
		}
		if it.Kind == model.KindError && it.Value != nil {
			t.Error("error item must never carry a byte value")
		}
		if it.Kind == model.KindData && *it.Value == 0x55 && it.StartNs == frame0Start*periodNs {
			t.Error("frame interrupted by the gap must never decode as 0x55")
		}
	}
	if !hasGapErr {
		t.Error("want a sample_gap error for the interrupted frame")
	}

	// Scenario 2: gap during idle between frame 0 end and frame 1 start.
	frame0End := frame0Start + 10*spb
	idleSplit := frame0End + spb/2
	c1 := model.SampleBlock{ConfigVersion: b.ConfigVersion, StartIndex: 0,
		Count: int(idleSplit), Levels: b.Levels[:idleSplit]}
	c2 := model.SampleBlock{ConfigVersion: b.ConfigVersion, StartIndex: idleSplit + 5*spb,
		Count: b.Count - int(idleSplit), Levels: b.Levels[idleSplit:]}
	items2 := decodeAll(t, uartCfg(), sampleCfg, []model.SampleBlock{c1, c2})
	var vals []int
	for _, it := range items2 {
		if it.Kind != model.KindData {
			t.Errorf("idle-period gap must not fabricate errors: %+v", it)
		}
		if it.Kind == model.KindData {
			vals = append(vals, *it.Value)
		}
	}
	if len(vals) != 2 || vals[0] != 0x55 {
		t.Errorf("frames across an idle gap = %v, want [85 ...]", vals)
	}
}

// A stretched bit cell moves an edge off the bit grid: clock anomaly error.
func TestClockAnomaly(t *testing.T) {
	sampleCfg := baseSampleCfg()
	blocks := demo.Generate(sampleCfg, demo.Params{
		Channel: "tx", BaudRate: 9600, Bytes: []byte{0x55},
		GlitchFrame: 0, GlitchBit: 2, GlitchSamples: 2.5,
	})
	items := decodeAll(t, uartCfg(), sampleCfg, blocks)
	hasClock := false
	for _, it := range items {
		if it.Kind == model.KindError && it.ErrCode == model.ErrClockAnomaly {
			hasClock = true
		}
		if it.Kind == model.KindData {
			t.Error("glitched frame must not decode to a byte")
		}
	}
	if !hasClock {
		t.Error("want clock_anomaly error")
	}
}

func TestFramingAndParity(t *testing.T) {
	sampleCfg := baseSampleCfg()

	// Even parity of 0x41 (2 ones) needs parity bit 0; ask for odd -> error.
	blocks := demo.Generate(sampleCfg, demo.Params{
		Channel: "tx", BaudRate: 9600, DataBits: 8, Parity: "even", Bytes: []byte{0x41},
	})
	oddCfg := uartCfg()
	oddCfg.Parity = "odd"
	items := decodeAll(t, oddCfg, sampleCfg, blocks)
	found := false
	for _, it := range items {
		if it.Kind == model.KindError && it.ErrCode == model.ErrParity {
			found = true
		}
	}
	if !found {
		t.Error("want parity error")
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
