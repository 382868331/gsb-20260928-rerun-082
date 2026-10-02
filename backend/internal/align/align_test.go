package align

import (
	"testing"

	"logicstation/internal/model"
)

func anchors(pairs ...[2]int64) []model.AlignmentAnchor {
	out := []model.AlignmentAnchor{}
	for i, pair := range pairs {
		out = append(out, model.AlignmentAnchor{
			ID: string(rune('a' + i)), BaseNs: pair[0], OtherNs: pair[1],
		})
	}
	return out
}

func TestEstimateSingleAnchor(t *testing.T) {
	est := EstimateOffsets(anchors([2]int64{1000, 5000}))
	if est.OffsetNs != 4000 || est.HasDrift {
		t.Fatalf("single anchor: offset=%d hasDrift=%v", est.OffsetNs, est.HasDrift)
	}
	mapped := est.MapInterval(1000, 2000)
	if !mapped.Mapped || mapped.OtherStartNs != 5000 || mapped.OtherEndNs != 6000 {
		t.Fatalf("mapped=%+v", mapped)
	}
}

func TestEstimateDriftAndConfidence(t *testing.T) {
	// 漂移变化：前半段 +0 偏移增长，中段锚点偏离直线。
	est := EstimateOffsets(anchors(
		[2]int64{0, 1000},
		[2]int64{1000000, 1001000},
		[2]int64{2000000, 2000800},
	))
	if !est.HasDrift {
		t.Fatal("两个以上锚点应估计漂移")
	}
	if est.ConfidenceNs <= 0 {
		t.Fatal("漂移变化应产生非零可信范围")
	}
	if len(est.Anchors) != 3 {
		t.Fatalf("anchors=%d", len(est.Anchors))
	}
	mapped := est.MapInterval(500000, 600000)
	if !mapped.Mapped {
		t.Fatal("覆盖范围内应可映射")
	}
	if mapped.HighNs-mapped.LowNs < 2*est.ConfidenceNs {
		t.Fatal("映射区间应附带可信范围")
	}
	if outside := est.MapInterval(3000000, 3001000); outside.Mapped {
		t.Fatal("覆盖范围之外的片段必须保留为无法对应")
	}
}

func TestCandidatesExcludeRepeated(t *testing.T) {
	v := func(b byte) *byte { return &b }
	base := []model.Marker{
		{ID: "b1", Kind: "byte", Value: v('H'), StartNs: 0},
		{ID: "b2", Kind: "byte", Value: v('L'), StartNs: 100},
		{ID: "b3", Kind: "byte", Value: v('L'), StartNs: 200},
		{ID: "b4", Kind: "byte", Value: v('Q'), StartNs: 300},
	}
	other := []model.Marker{
		{ID: "o1", Kind: "byte", Value: v('H'), StartNs: 1000},
		{ID: "o2", Kind: "byte", Value: v('L'), StartNs: 1100},
		{ID: "o3", Kind: "byte", Value: v('L'), StartNs: 1200},
	}
	accepted, excluded := AnchorCandidates(base, other)
	if len(accepted) != 1 || accepted[0].OtherMarkerID != "o1" {
		t.Fatalf("accepted=%+v", accepted)
	}
	if len(excluded) != 3 {
		t.Fatalf("excluded=%+v", excluded)
	}
	for _, candidate := range excluded {
		if candidate.Reason == "" {
			t.Fatal("被排除的候选必须记录原因")
		}
	}
}

func TestCompareClassifiesCauses(t *testing.T) {
	v := func(b byte) *byte { return &b }
	cfg := model.Config{Protocol: "uart", Parameters: model.UARTParameters{Baud: 9600, DataBits: 8, StopBits: 1}}
	otherCfg := cfg
	base := model.Result{Markers: []model.Marker{
		{ID: "b1", Kind: "byte", Value: v('H'), StartNs: 0, EndNs: 100},
		{ID: "b2", Kind: "byte", Value: v('L'), StartNs: 1000, EndNs: 1100},
		{ID: "b3", Kind: "byte", Value: v('O'), StartNs: 1900, EndNs: 2000},
		{ID: "b4", Kind: "byte", Value: v('X'), StartNs: 9000, EndNs: 9100},
	}}
	other := model.Result{Markers: []model.Marker{
		{ID: "o1", Kind: "byte", Value: v('H'), StartNs: 5000, EndNs: 5100},
		{ID: "o2", Kind: "error", Code: "sampling-gap", StartNs: 6000, EndNs: 6150},
		{ID: "o3", Kind: "byte", Value: v('P'), StartNs: 6900, EndNs: 7000},
	}}
	alignment := model.Alignment{
		ID: "g:v1", Version: 1,
		Anchors: anchors([2]int64{0, 5000}, [2]int64{2000, 7000}),
	}
	report := CompareResults(alignment, base, other, cfg, otherCfg)
	if report.Matched != 1 {
		t.Fatalf("matched=%d", report.Matched)
	}
	causes := map[string]string{}
	for _, diff := range report.Diffs {
		if diff.Base != nil {
			causes[diff.Base.ID] = diff.Cause
		}
	}
	if causes["b2"] != CauseSamplingGap {
		t.Fatalf("b2 cause=%s, want sampling-gap", causes["b2"])
	}
	if causes["b3"] != CauseSignalDifference {
		t.Fatalf("b3 cause=%s, want signal-difference", causes["b3"])
	}
	if causes["b4"] != CauseUnmapped {
		t.Fatalf("b4 cause=%s, want unmapped（覆盖范围外不强行对齐）", causes["b4"])
	}
	for _, diff := range report.Diffs {
		if diff.Base != nil && diff.Base.ID == "b3" {
			if diff.OtherStartNs != 6900 || diff.OtherEndNs != 7000 {
				t.Fatalf("异常字节应映射到另一份采样的可能区间: %+v", diff)
			}
		}
	}
}

func TestCompareDecodeConfigCause(t *testing.T) {
	v := func(b byte) *byte { return &b }
	baseCfg := model.Config{Protocol: "uart", Parameters: model.UARTParameters{Baud: 9600, DataBits: 8, StopBits: 1}}
	otherCfg := model.Config{Protocol: "uart", Parameters: model.UARTParameters{Baud: 4800, DataBits: 8, StopBits: 1}}
	base := model.Result{Markers: []model.Marker{{ID: "b1", Kind: "byte", Value: v('A'), StartNs: 0, EndNs: 100}}}
	other := model.Result{Markers: []model.Marker{{ID: "o1", Kind: "byte", Value: v('B'), StartNs: 5000, EndNs: 5100}}}
	alignment := model.Alignment{ID: "g:v1", Version: 1, Anchors: anchors([2]int64{0, 5000}, [2]int64{1000, 6000})}
	report := CompareResults(alignment, base, other, baseCfg, otherCfg)
	if len(report.Diffs) != 1 || report.Diffs[0].Cause != CauseDecodeConfig {
		t.Fatalf("diffs=%+v", report.Diffs)
	}
}
