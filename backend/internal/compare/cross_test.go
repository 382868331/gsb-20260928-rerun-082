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
