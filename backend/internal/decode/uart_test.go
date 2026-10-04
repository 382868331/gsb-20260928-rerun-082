package decode

import (
	"testing"

	"logicstation/internal/model"
)

func makeUARTSamples(value byte, baud, overSample int) []uint8 {
	bit := overSample
	samples := make([]uint8, bit*13)
	for i := range samples {
		samples[i] = 1
	}
	offset := bit * 2
	for sample := offset; sample < offset+bit; sample++ {
		samples[sample] = 0
	}
	for bitIndex := 0; bitIndex < 8; bitIndex++ {
		var level uint8
		if value&(1<<bitIndex) != 0 {
			level = 1
		}
		for sample := offset + (bitIndex+1)*bit; sample < offset+(bitIndex+2)*bit; sample++ {
			samples[sample] = level
		}
	}
	for sample := offset + 9*bit; sample < len(samples); sample++ {
		samples[sample] = 1
	}
	return samples
}

func sampleChunks(baud int, split int, first []uint8, second []uint8, gap bool) []model.Chunk {
	interval := int64(1000000000 / (baud * 16))
	toRows := func(values []uint8) [][]uint8 {
		rows := make([][]uint8, len(values))
		for i, value := range values {
			rows[i] = []uint8{value}
		}
		return rows
	}
	secondStart := int64(split) * interval
	if gap {
		secondStart += interval * 20
	}
	return []model.Chunk{
		{ID: "c0", Index: 0, StartTime: 0, IntervalNs: interval, Levels: toRows(first)},
		{ID: "c1", Index: 1, StartTime: secondStart, IntervalNs: interval, Levels: toRows(second)},
	}
}

func testConfig() model.Config {
	return model.Config{
		ID: "cfg", CaptureID: "cap", Version: 1, Protocol: "uart",
		Parameters: model.UARTParameters{Channel: 0, Baud: 9600, DataBits: 8, Parity: "none", StopBits: 1, IdleLevel: 1, ClockTolerance: 0.03},
	}
}

func TestFrameContinuesAcrossChunkBoundary(t *testing.T) {
	levels := makeUARTSamples('A', 9600, 16)
	chunks := sampleChunks(9600, 140, levels[:140], levels[140:], false)
	result := (&UARTDecoder{}).Decode("test", chunks, testConfig())

	if result.ErrorCount != 0 {
		t.Fatalf("expected no errors, got %d: %#v", result.ErrorCount, result.Markers)
	}
	if result.ValidBytes != 1 || result.Markers[0].Value == nil || *result.Markers[0].Value != 'A' {
		t.Fatalf("expected byte A, got %#v", result.Markers)
	}
	interval := int64(1000000000 / (9600 * 16))
	if result.Markers[0].StartNs != 32*interval {
		t.Fatalf("unexpected start edge: %d", result.Markers[0].StartNs)
	}
}

func TestSamplingGapDoesNotBecomeByte(t *testing.T) {
	levels := makeUARTSamples('A', 9600, 16)
	chunks := sampleChunks(9600, 140, levels[:140], levels[140:], true)
	result := (&UARTDecoder{}).Decode("test", chunks, testConfig())

	if result.ValidBytes != 0 {
		t.Fatalf("gap intersecting a frame must not decode a byte: %#v", result.Markers)
	}
	if len(result.Markers) == 0 || result.Markers[0].Code != "sampling-gap" {
		t.Fatalf("expected a sampling-gap error, got %#v", result.Markers)
	}
}

func TestFramingErrorIsPreserved(t *testing.T) {
	levels := makeUARTSamples('X', 9600, 16)
	for i := 11 * 16; i < 13*16; i++ {
		levels[i] = 0
	}
	chunks := sampleChunks(9600, len(levels), levels, nil, false)
	result := (&UARTDecoder{}).Decode("test", chunks[:1], testConfig())

	if result.ValidBytes != 0 || result.ErrorCount != 1 || result.Markers[0].Code != "framing-error" {
		t.Fatalf("expected framing error without a normal byte, got %#v", result.Markers)
	}
}

func TestClockAnomalyIsPreserved(t *testing.T) {
	levels := makeUARTSamples('X', 9600, 16)
	// A return to idle in the middle of a low data bit is not a legal UART bit boundary.
	for i := 2*16 + 3*16 + 4; i < 2*16+3*16+8; i++ {
		levels[i] = 1
	}
	chunks := sampleChunks(9600, len(levels), levels, nil, false)
	result := (&UARTDecoder{}).Decode("test", chunks[:1], testConfig())

	if result.ValidBytes != 0 || result.ErrorCount != 1 || result.Markers[0].Code != "clock-anomaly" {
		t.Fatalf("expected clock anomaly without a normal byte, got %#v", result.Markers)
	}
}
func TestClockResetSplitsCaptureIntoSegments(t *testing.T) {
	type edge struct {
		at    int64
		level uint8
	}
	// 同一逻辑采样中途两次重置时钟：后半段时间戳重新从较小值开始。
	// 解码必须保留采集段，重置前后的相同时间戳是不同事件。
	bit := int64(10000)
	interval := int64(2500)
	byteAt := func(value byte, start int64) []edge {
		changes := []edge{{start, 0}}
		for i := 0; i < 8; i++ {
			changes = append(changes, edge{start + bit*int64(i+1), (value >> i) & 1})
		}
		return append(changes, edge{start + bit*9, 1})
	}
	render := func(events []edge, end int64) [][]uint8 {
		rows := [][]uint8{}
		for ts := int64(0); ts <= end; ts += interval {
			level := uint8(1)
			for _, c := range events {
				if c.at <= ts+interval/2 {
					level = c.level
				}
			}
			rows = append(rows, []uint8{0, level})
		}
		return rows
	}
	seg0 := append(byteAt('A', 20000), byteAt('B', 150000)...)
	seg1 := append(byteAt('A', 20000), byteAt('C', 150000)...)
	seg2 := byteAt('A', 20000)
	chunks := []model.Chunk{
		{Index: 0, StartTime: 0, IntervalNs: interval, Levels: render(seg0, 260000)},
		{Index: 1, StartTime: 0, IntervalNs: interval, Levels: render(seg1, 260000)},
		{Index: 2, StartTime: 0, IntervalNs: interval, Levels: render(seg2, 260000)},
	}
	cfg := model.Config{ID: "cfg-reset", CaptureID: "cap", Protocol: "uart", Parameters: model.UARTParameters{Channel: 1, Baud: 100000, DataBits: 8, Parity: "none", StopBits: 1, IdleLevel: 1, ClockTolerance: 0.04}}
	result := (&UARTDecoder{}).Decode("reset", chunks, cfg)
	resets := 0
	bytesPerSegment := map[int][]byte{}
	for _, m := range result.Markers {
		if m.Code == "clock-reset" {
			resets++
		}
		if m.Kind == "byte" {
			bytesPerSegment[m.SegmentIndex] = append(bytesPerSegment[m.SegmentIndex], *m.Value)
		}
	}
	if resets != 2 {
		t.Fatalf("clock-reset markers=%d, want 2: %+v", resets, result.Markers)
	}
	if len(bytesPerSegment) != 3 || len(bytesPerSegment[0]) != 2 || len(bytesPerSegment[1]) != 2 || len(bytesPerSegment[2]) != 1 {
		t.Fatalf("bytes per segment=%v", bytesPerSegment)
	}
	// 三段各自的 A 都在原生 20000ns，但属于不同采集段的不同事件。
	for segment := 0; segment < 3; segment++ {
		if bytesPerSegment[segment][0] != 'A' {
			t.Fatalf("segment %d first byte=%q", segment, bytesPerSegment[segment][0])
		}
	}
}
