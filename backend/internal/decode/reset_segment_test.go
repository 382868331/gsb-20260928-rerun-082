package decode

import (
	"testing"

	"logicstation/internal/model"
)

func TestDecoderLabelsClockResetChunksWithoutGap(t *testing.T) {
	chunks := []model.Chunk{
		{ID: "a", CaptureID: "c", Index: 0, SegmentID: "segment-1", StartTime: 0, IntervalNs: 1000, Levels: [][]uint8{{1}, {1}, {1}, {1}}},
		{ID: "b", CaptureID: "c", Index: 1, SegmentID: "segment-2", ClockReset: true, StartTime: 0, IntervalNs: 1000, Levels: [][]uint8{{1}, {1}, {1}, {1}}},
	}
	cfg := model.Config{ID: "cfg", CaptureID: "c", Protocol: "uart", Parameters: model.UARTParameters{Channel: 0, Baud: 9600, DataBits: 8, Parity: "none", StopBits: 1, IdleLevel: 1}}
	result := (&UARTDecoder{}).Decode("c", chunks, cfg)
	hasReset := false
	for _, marker := range result.Markers {
		if marker.Code == "clock-reset" && marker.SegmentID == "segment-2" {
			hasReset = true
		}
		if marker.Code == "sampling-gap" {
			t.Fatalf("clock reset must not be classified as a missing sample: %+v", marker)
		}
	}
	if len(chunks) != 2 || chunks[1].SegmentID != "segment-2" {
		t.Fatalf("segment identity not retained: %+v", chunks)
	}
	if !hasReset {
		t.Fatalf("expected clock-reset marker in segment-2: %+v", result.Markers)
	}
}
