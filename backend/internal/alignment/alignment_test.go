package alignment

import (
	"strings"
	"testing"

	"logicstation/internal/model"
)

func chunksWithTwoResets() []model.Chunk {
	return []model.Chunk{
		{ID: "b1", Index: 0, StartTime: 0, IntervalNs: 10, Levels: make([][]uint8, 20)},
		{ID: "b2", Index: 1, StartTime: 5, IntervalNs: 10, Levels: make([][]uint8, 20)},
		{ID: "b3", Index: 2, StartTime: 2, IntervalNs: 10, Levels: make([][]uint8, 20)},
	}
}

func TestClockResetSegmentsKeepSameTimestampDistinct(t *testing.T) {
	model.NormalizeChunkSegments(chunksWithTwoResets())
	baseChunks := []model.Chunk{
		{ID: "base-1", Index: 0, SegmentID: "segment-1", StartTime: 100, IntervalNs: 10, Levels: make([][]uint8, 20)},
		{ID: "base-2", Index: 1, SegmentID: "segment-2", StartTime: 200, IntervalNs: 10, Levels: make([][]uint8, 20)},
	}
	otherChunks := chunksWithTwoResets()
	align := model.Alignment{ID: "a", BaseCaptureID: "base", OtherCaptureID: "other", Anchors: []model.AlignmentAnchor{
		{ID: "a1", Sequence: 1, BaseMarkerID: "b1", OtherMarkerID: "o1", BaseSegmentID: "segment-1", OtherSegmentID: "segment-1", BaseTimeNs: 100, OtherTimeNs: 10, UncertaintyNs: 1},
		{ID: "a2", Sequence: 2, BaseMarkerID: "b2", OtherMarkerID: "o2", BaseSegmentID: "segment-1", OtherSegmentID: "segment-1", BaseTimeNs: 150, OtherTimeNs: 60, UncertaintyNs: 1},
		{ID: "a3", Sequence: 3, BaseMarkerID: "b3", OtherMarkerID: "o3", BaseSegmentID: "segment-2", OtherSegmentID: "segment-2", BaseTimeNs: 200, OtherTimeNs: 5, UncertaintyNs: 1},
		{ID: "a4", Sequence: 4, BaseMarkerID: "b4", OtherMarkerID: "o4", BaseSegmentID: "segment-2", OtherSegmentID: "segment-2", BaseTimeNs: 250, OtherTimeNs: 55, UncertaintyNs: 1},
	}}
	_, mapping, err := Build(align, baseChunks, otherChunks)
	if err != nil {
		t.Fatal(err)
	}
	first, ok := mapping.MapInterval("segment-1", 10, 10, 1)
	if !ok || first.Start != 100 {
		t.Fatalf("first segment mapping=%+v ok=%v", first, ok)
	}
	second, ok := mapping.MapInterval("segment-2", 10, 10, 1)
	if !ok || second.Start != 205 {
		t.Fatalf("second reset round must use its own mapping: %+v ok=%v", second, ok)
	}
	if _, ok := mapping.MapInterval("segment-3", 10, 10, 1); ok {
		t.Fatal("unanchored third segment must not borrow another reset round")
	}
}

func TestManualOrderConflictNamesAnchorAndSegments(t *testing.T) {
	chunks := chunksWithTwoResets()
	align := model.Alignment{ID: "a", BaseCaptureID: "base", OtherCaptureID: "other", Anchors: []model.AlignmentAnchor{
		{ID: "later", Sequence: 1, BaseMarkerID: "b1", OtherMarkerID: "o1", BaseSegmentID: "segment-2", OtherSegmentID: "segment-2", BaseTimeNs: 200, OtherTimeNs: 10, UncertaintyNs: 1, ManualOrderConfirmed: true},
		{ID: "earlier", Sequence: 2, BaseMarkerID: "b2", OtherMarkerID: "o2", BaseSegmentID: "segment-1", OtherSegmentID: "segment-1", BaseTimeNs: 100, OtherTimeNs: 5, UncertaintyNs: 1, ManualOrderConfirmed: true},
	}}
	got, _, err := Build(align, chunks, chunks)
	if err == nil || got.Status != StatusConflict {
		t.Fatalf("expected conflict, got err=%v alignment=%+v", err, got)
	}
	message := err.Error()
	for _, want := range []string{"later", "earlier", "segment-1", "segment-2"} {
		if !strings.Contains(message, want) {
			t.Fatalf("conflict %q missing %q", message, want)
		}
	}
}
