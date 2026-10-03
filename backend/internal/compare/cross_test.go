package compare

import (
	"testing"

	"logicstation/internal/alignment"
	"logicstation/internal/model"
)

func TestCrossResultsClassifiesProtocolParameters(t *testing.T) {
	baseMarker := byteMarker("b1", 100, 200, 0x48, "01001000")
	otherMarker := byteMarker("o1", 50, 150, 0x12, "01001000")
	base := model.Result{ID: "base", CaptureID: "c1", Markers: []model.Marker{baseMarker}}
	other := model.Result{ID: "other", CaptureID: "c2", Markers: []model.Marker{otherMarker}}
	align := model.Alignment{
		ID: "a1", Status: alignment.StatusReady, BaseResultID: "base", OtherResultID: "other",
		Anchors: []model.AlignmentAnchor{{ID: "x", BaseMarkerID: "b1", OtherMarkerID: "o1", BaseTimeNs: 100, OtherTimeNs: 50, UncertaintyNs: 5}},
	}
	chunksA := []model.Chunk{{StartTime: 0, IntervalNs: 10, Levels: make([][]uint8, 100)}}
	chunksB := []model.Chunk{{StartTime: 0, IntervalNs: 10, Levels: make([][]uint8, 100)}}
	_, mapping, err := alignment.Build(align, chunksA, chunksB)
	if err != nil {
		t.Fatal(err)
	}
	report := CrossResults(align, mapping, base, other, spiCfg("rising", "msb"), spiCfg("rising", "lsb"), chunksA, chunksB)
	if report.Identical || len(report.Diffs) != 1 || report.Diffs[0].Cause != CauseParameters {
		t.Fatalf("report=%+v", report)
	}
}

func TestCrossResultsDoesNotMatchDuplicateAcrossResetRounds(t *testing.T) {
	base := model.Result{ID: "base", CaptureID: "c1", Markers: []model.Marker{
		byteMarkerWithSegment("b1", "segment-1", 100, 200, 'A'),
		byteMarkerWithSegment("b2", "segment-2", 300, 400, 'A'),
	}}
	other := model.Result{ID: "other", CaptureID: "c2", Markers: []model.Marker{
		byteMarkerWithSegment("o1", "segment-1", 50, 150, 'A'),
		byteMarkerWithSegment("o2", "segment-2", 50, 150, 'A'),
	}}
	align := model.Alignment{ID: "a", Status: alignment.StatusReady, BaseResultID: "base", OtherResultID: "other", Anchors: []model.AlignmentAnchor{
		{ID: "x1", Sequence: 1, BaseMarkerID: "b1", OtherMarkerID: "o1", BaseSegmentID: "segment-1", OtherSegmentID: "segment-1", BaseTimeNs: 100, OtherTimeNs: 50, UncertaintyNs: 5},
		{ID: "x2", Sequence: 2, BaseMarkerID: "b2", OtherMarkerID: "o2", BaseSegmentID: "segment-2", OtherSegmentID: "segment-2", BaseTimeNs: 300, OtherTimeNs: 50, UncertaintyNs: 5},
	}}
	chunks := []model.Chunk{
		{ID: "s1", Index: 0, SegmentID: "segment-1", StartTime: 0, IntervalNs: 10, Levels: make([][]uint8, 100)},
		{ID: "s2", Index: 1, SegmentID: "segment-2", ClockReset: true, StartTime: 0, IntervalNs: 10, Levels: make([][]uint8, 100)},
	}
	_, mapping, err := alignment.Build(align, chunks, chunks)
	if err != nil {
		t.Fatal(err)
	}
	report := CrossResults(align, mapping, base, other, uartCfg(), uartCfg(), chunks, chunks)
	if !report.Identical || len(report.Diffs) != 0 {
		t.Fatalf("same segment duplicate must pair, cross-round timestamp must not compete: %+v", report)
	}
}

func byteMarkerWithSegment(id, segment string, start, end int64, value byte) model.Marker {
	m := byteMarker(id, start, end, value, "01000001")
	m.SegmentID = segment
	return m
}

func uartCfg() model.Config {
	return model.Config{Protocol: "uart", Parameters: model.UARTParameters{Baud: 9600}}
}
