package model

import "testing"

func TestNormalizeChunksDetectsWrapAndTwoResets(t *testing.T) {
	chunks := []Chunk{
		{ID: "a", Index: 0, StartTime: 100, IntervalNs: 10, Levels: make([][]uint8, 2)},
		{ID: "b", Index: 1, StartTime: 50, IntervalNs: 10, Levels: make([][]uint8, 2)},
		{ID: "c", Index: 2, StartTime: 20, IntervalNs: 10, Levels: make([][]uint8, 2)},
	}
	NormalizeChunkSegments(chunks)
	for i, want := range []struct {
		segment string
		reset   bool
	}{{"segment-1", false}, {"segment-2", true}, {"segment-3", true}} {
		if chunks[i].SegmentID != want.segment || chunks[i].ClockReset != want.reset {
			t.Fatalf("chunk %d = %s/%v, want %s/%v", i, chunks[i].SegmentID, chunks[i].ClockReset, want.segment, want.reset)
		}
	}
	segments := CaptureSegments("capture", chunks)
	if len(segments) != 3 || segments[2].StartNs != 20 {
		t.Fatalf("segments=%+v", segments)
	}
}
