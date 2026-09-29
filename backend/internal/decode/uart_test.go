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
