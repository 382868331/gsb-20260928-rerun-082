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

func resetByteMarker(id string, segment int, start int64, value byte) model.Marker {
	m := byteMarker(id, start, start+10000, value, "01001000")
	m.SegmentIndex = segment
	return m
}

// 采集中途时钟重置：每个采集段独立与另一份采样对齐，重复报文保留歧义，
// 缺证据的段保持 unmatched，内容一致但残差超限解释为时钟漂移。
func TestCrossResultsWithClockResetSegments(t *testing.T) {
	base := model.Result{ID: "base", CaptureID: "c1", Markers: []model.Marker{
		resetByteMarker("bA0", 0, 10000, 'A'),
		resetByteMarker("bB0", 0, 40000, 'B'),
		resetByteMarker("bA1", 1, 10000, 'A'),
		resetByteMarker("bC1", 1, 40000, 'C'),
		resetByteMarker("bD2", 2, 10000, 'D'),
	}}
	other := model.Result{ID: "other", CaptureID: "c2", Markers: []model.Marker{
		resetByteMarker("oA0", 0, 50000, 'A'),
		resetByteMarker("oB0", 0, 82000, 'B'), // 残差 2000ns：时钟漂移
		resetByteMarker("oA1", 0, 300000, 'A'),
		func() model.Marker {
			m := resetByteMarker("oX1", 0, 330000, 'X') // 内容真实不同
			m.RawBits = "01011000"
			return m
		}(),
	}}
	align := model.Alignment{
		ID: "a1", Status: alignment.StatusReady, BaseResultID: "base", OtherResultID: "other",
		Anchors: []model.AlignmentAnchor{
			{ID: "k1", BaseMarkerID: "bA0", OtherMarkerID: "oA0", BaseTimeNs: 10000, OtherTimeNs: 50000, UncertaintyNs: 10, BaseSegmentIndex: 0, OtherSegmentIndex: 0},
			{ID: "k2", BaseMarkerID: "bA1", OtherMarkerID: "oA1", BaseTimeNs: 10000, OtherTimeNs: 300000, UncertaintyNs: 10, BaseSegmentIndex: 1, OtherSegmentIndex: 0},
		},
	}
	baseChunks := []model.Chunk{
		{ID: "c0", Index: 0, StartTime: 0, IntervalNs: 100, Levels: make([][]uint8, 1001)},
		{ID: "c1", Index: 1, StartTime: 0, IntervalNs: 100, Levels: make([][]uint8, 1001)},
		{ID: "c2", Index: 2, StartTime: 0, IntervalNs: 100, Levels: make([][]uint8, 1001)},
	}
	otherChunks := []model.Chunk{{ID: "o0", Index: 0, StartTime: 0, IntervalNs: 100, Levels: make([][]uint8, 5001)}}
	built, mapping, err := alignment.Build(align, baseChunks, otherChunks)
	if err != nil {
		t.Fatal(err)
	}
	cfg := spiCfg("rising", "msb")
	report := CrossResults(built, mapping, base, other, cfg, cfg, baseChunks, otherChunks)

	byBase := map[string]*CrossMarkerDiff{}
	var drift, signal *CrossMarkerDiff
	for i := range report.Diffs {
		d := &report.Diffs[i]
		if d.Base != nil {
			byBase[d.Base.ID] = d
		}
		switch d.Cause {
		case CauseClockDrift:
			drift = d
		case CauseSignal:
			signal = d
		}
	}
	// 段 2 没有锚点证据：保持 unmatched，绝不借用其他段的映射拉伸。
	orphan, ok := byBase["bD2"]
	if !ok || orphan.Cause != CauseUnmatched || orphan.BaseSegmentIndex != 2 {
		t.Fatalf("segment 2 marker must stay unmatched with its segment: %+v", report.Diffs)
	}
	// 内容真实不同（C vs X）归为 actual-signal，且证据锚点可追溯到段 1 的 k2。
	if signal == nil || signal.Base == nil || signal.Base.ID != "bC1" {
		t.Fatalf("expected actual-signal diff for C vs X: %+v", report.Diffs)
	}
	if signal.BaseSegmentIndex != 1 || len(signal.EvidenceAnchorIDs) == 0 || signal.EvidenceAnchorIDs[0] != "k2" {
		t.Fatalf("signal diff must trace to segment 1 anchor k2: %+v", signal)
	}
	// 内容一致但残差超限：解释为时钟漂移，而不是信号差异。
	if drift == nil || drift.Base == nil || drift.Base.ID != "bB0" {
		t.Fatalf("expected clock-drift diff for B: %+v", report.Diffs)
	}
	// 从段 1 的异常字节反查另一采样：必须落在第二轮（约 330000），
	// 不能进入第一轮同名时间区间（约 40000）。
	if signal.OtherNativeStartNs < 200000 {
		t.Fatalf("cross-segment jump landed in wrong round: %+v", signal)
	}
	if report.DriftCount != 1 || report.DiffCount != 1 || report.UnmatchedCount != 1 {
		t.Fatalf("counts drift=%d diff=%d unmatched=%d", report.DriftCount, report.DiffCount, report.UnmatchedCount)
	}
}
