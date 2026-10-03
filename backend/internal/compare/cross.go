package compare

import (
	"math"
	"sort"

	"logicstation/internal/alignment"
	"logicstation/internal/model"
)

const (
	CauseSignal          = "actual-signal"
	CauseSamplingMissing = "sampling-missing"
	CauseClockDrift      = "clock-drift"
	CauseParameters      = "protocol-parameters"
	CauseUnmatched       = "unmatched"
)

type Coverage struct {
	StartNs int64 `json:"startNs"`
	EndNs   int64 `json:"endNs"`
}

type CrossMarkerDiff struct {
	Base               *model.Marker `json:"base,omitempty"`
	Other              *model.Marker `json:"other,omitempty"`
	BaseSegmentID      string        `json:"baseSegmentId,omitempty"`
	OtherSegmentID     string        `json:"otherSegmentId,omitempty"`
	BaseChunkID        string        `json:"baseChunkId,omitempty"`
	OtherChunkID       string        `json:"otherChunkId,omitempty"`
	AlignmentID        string        `json:"alignmentId,omitempty"`
	AnchorIDs          []string      `json:"anchorIds,omitempty"`
	MappedStartNs      int64         `json:"mappedStartNs,omitempty"`
	MappedEndNs        int64         `json:"mappedEndNs,omitempty"`
	OtherNativeStartNs int64         `json:"otherNativeStartNs,omitempty"`
	OtherNativeEndNs   int64         `json:"otherNativeEndNs,omitempty"`
	ConfidenceHalfNs   int64         `json:"confidenceHalfWidthNs"`
	Cause              string        `json:"cause"`
	CauseText          string        `json:"causeText"`
	Detail             string        `json:"detail"`
}

type CrossReport struct {
	AlignmentID    string                    `json:"alignmentId"`
	BaseResultID   string                    `json:"baseResultId"`
	OtherResultID  string                    `json:"otherResultId"`
	Status         string                    `json:"status"`
	OffsetSummary  string                    `json:"offsetSummary,omitempty"`
	Segments       []alignment.Segment       `json:"segments"`
	Conflicts      []model.AlignmentConflict `json:"conflicts,omitempty"`
	Identical      bool                      `json:"identical"`
	DiffCount      int                       `json:"diffCount"`
	UnmatchedCount int                       `json:"unmatchedCount"`
	Diffs          []CrossMarkerDiff         `json:"diffs"`
	OtherLocations []CrossMarkerDiff         `json:"otherLocations"`
}

type matched struct {
	base   *model.Marker
	other  *model.Marker
	mapped alignment.Interval
}

func CrossResults(align model.Alignment, mapping alignment.Mapping, base, other model.Result, baseCfg, otherCfg model.Config, baseChunks, otherChunks []model.Chunk) CrossReport {
	report := CrossReport{
		AlignmentID: align.ID, BaseResultID: base.ID, OtherResultID: other.ID, Status: align.Status,
		Segments: mapping.Segments, Conflicts: align.Conflicts, Diffs: []CrossMarkerDiff{}, OtherLocations: []CrossMarkerDiff{},
	}
	report.OffsetSummary = mapping.SegmentSummary()
	if align.Status != alignment.StatusReady {
		return report
	}
	baseCoverage := cover(baseChunks)
	otherCoverage := mappedCover(otherChunks, mapping)
	baseGap := mappedGapCoverage(other, mapping)
	otherGap := coverGaps(base)
	usedBase := map[int]bool{}
	usedOther := map[int]bool{}
	pairs := []matched{}
	ambiguous := map[int]bool{}
	anchoredBase := map[string]bool{}
	anchoredOther := map[string]bool{}
	for _, a := range align.Anchors {
		anchoredBase[a.BaseMarkerID] = true
		anchoredOther[a.OtherMarkerID] = true
		var bm, om *model.Marker
		for i := range base.Markers {
			if base.Markers[i].ID == a.BaseMarkerID {
				bm = &base.Markers[i]
			}
		}
		for i := range other.Markers {
			if other.Markers[i].ID == a.OtherMarkerID {
				om = &other.Markers[i]
			}
		}
		if bm != nil && om != nil {
			mapped, _ := mapping.MapInterval(om.SegmentID, om.StartNs, om.EndNs, uncertaintyFor(om, otherChunks))
			pairs = append(pairs, matched{base: bm, other: om, mapped: mapped})
		}
	}

	for oi := range other.Markers {
		om := &other.Markers[oi]
		if anchoredOther[om.ID] {
			continue
		}
		mapped, ok := mapping.MapInterval(om.SegmentID, om.StartNs, om.EndNs, uncertaintyFor(om, otherChunks))
		if !ok {
			continue
		}
		candidates := []int{}
		for bi := range base.Markers {
			bm := &base.Markers[bi]
			if anchoredBase[bm.ID] {
				continue
			}
			if bm.SegmentID != mapped.BaseSegmentID {
				continue
			}
			iv := alignment.Interval{Start: bm.StartNs, End: bm.EndNs, Half: uncertaintyFor(bm, baseChunks)}
			if iv.Overlaps(mapped) {
				candidates = append(candidates, bi)
			}
		}
		if len(candidates) == 1 && !anchoredBase[base.Markers[candidates[0]].ID] {
			pairs = append(pairs, matched{base: &base.Markers[candidates[0]], other: om, mapped: mapped})
		} else if len(candidates) > 1 {
			for _, id := range candidates {
				ambiguous[id] = true
			}
			report.Diffs = append(report.Diffs, CrossMarkerDiff{Other: om, MappedStartNs: mapped.Start, MappedEndNs: mapped.End, ConfidenceHalfNs: mapped.Half, Cause: CauseUnmatched, CauseText: "无法唯一对应：多个基准事件落入可信范围", Detail: "保留该片段，不按最近距离强行拉伸匹配"})
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].base.StartNs < pairs[j].base.StartNs })
	for _, p := range pairs {
		bi := markerIndex(base.Markers, p.base.ID)
		oi := markerIndex(other.Markers, p.other.ID)
		if usedBase[bi] || usedOther[oi] || ambiguous[bi] {
			continue
		}
		usedBase[bi] = true
		usedOther[oi] = true
		if sameDecodedEvent(p.base, p.other) {
			baseCenter := (p.base.StartNs + p.base.EndNs) / 2
			mappedCenter := (p.mapped.Start + p.mapped.End) / 2
			tolerance := p.mapped.Half + uncertaintyFor(p.base, baseChunks)
			residual := baseCenter - mappedCenter
			if residual < 0 {
				residual = -residual
			}
			if residual > tolerance {
				diff := CrossMarkerDiff{Base: p.base, Other: p.other, MappedStartNs: p.mapped.Start, MappedEndNs: p.mapped.End, ConfidenceHalfNs: p.mapped.Half, Cause: CauseClockDrift, CauseText: "时钟漂移：解码事件相同，但段内时间残差超出锚点可信范围", Detail: "不把该时间偏差解释为协议内容或参数变化"}
				diff.AnchorIDs = anchorEvidence(align, p.base.ID, p.other.ID)
				diff.AlignmentID = align.ID
				report.Diffs = append(report.Diffs, withNativeRange(diff, mapping))
			}
			continue
		}
		diff := classifyCross(p.base, p.other, p.mapped, baseCfg, otherCfg)
		diff.AnchorIDs = anchorEvidence(align, p.base.ID, p.other.ID)
		diff.AlignmentID = align.ID
		report.Diffs = append(report.Diffs, withNativeRange(diff, mapping))
	}

	for bi := range base.Markers {
		if usedBase[bi] {
			continue
		}
		bm := &base.Markers[bi]
		diff := unmatchedOrMissing(bm, true, mapping, baseCoverage, otherCoverage, otherGap, baseChunks)
		diff = withNativeRange(diff, mapping)
		report.Diffs = append(report.Diffs, diff)
	}
	for oi := range other.Markers {
		if usedOther[oi] || other.Markers[oi].Code == "sampling-gap" {
			continue
		}
		om := &other.Markers[oi]
		if usedByBase(om.ID, base.Markers) {
			continue
		}
		diff := unmatchedOrMissing(om, false, mapping, baseCoverage, otherCoverage, baseGap, otherChunks)
		if mapped, ok := mapping.MapInterval(om.SegmentID, om.StartNs, om.EndNs, uncertaintyFor(om, otherChunks)); ok {
			diff.MappedStartNs, diff.MappedEndNs = mapped.Start, mapped.End
			diff.ConfidenceHalfNs = mapped.Half
			diff.BaseSegmentID = mapped.BaseSegmentID
			diff.AnchorIDs = mapped.AnchorIDs
		}
		report.Diffs = append(report.Diffs, diff)
	}

	for i := range other.Markers {
		m := &other.Markers[i]
		mapped, _ := mapping.MapInterval(m.SegmentID, m.StartNs, m.EndNs, uncertaintyFor(m, otherChunks))
		diff := CrossMarkerDiff{Other: m, MappedStartNs: mapped.Start, MappedEndNs: mapped.End, ConfidenceHalfNs: mapped.Half, AlignmentID: align.ID, OtherSegmentID: m.SegmentID, OtherChunkID: m.ChunkID, BaseSegmentID: mapped.BaseSegmentID, AnchorIDs: mapped.AnchorIDs}
		report.OtherLocations = append(report.OtherLocations, withNativeRange(diff, mapping))
	}
	sort.SliceStable(report.Diffs, func(i, j int) bool { return diffSortKey(report.Diffs[i]) < diffSortKey(report.Diffs[j]) })
	for i := range report.Diffs {
		report.Diffs[i].AlignmentID = align.ID
		if report.Diffs[i].BaseSegmentID == "" && report.Diffs[i].Base != nil {
			report.Diffs[i].BaseSegmentID = report.Diffs[i].Base.SegmentID
		}
		if report.Diffs[i].OtherSegmentID == "" && report.Diffs[i].Other != nil {
			report.Diffs[i].OtherSegmentID = report.Diffs[i].Other.SegmentID
		}
		if report.Diffs[i].BaseChunkID == "" && report.Diffs[i].Base != nil {
			report.Diffs[i].BaseChunkID = report.Diffs[i].Base.ChunkID
		}
		if report.Diffs[i].OtherChunkID == "" && report.Diffs[i].Other != nil {
			report.Diffs[i].OtherChunkID = report.Diffs[i].Other.ChunkID
		}
	}
	report.DiffCount = 0
	for _, d := range report.Diffs {
		if d.Cause != CauseUnmatched {
			report.DiffCount++
		}
	}
	report.UnmatchedCount = len(report.Diffs) - report.DiffCount
	report.Identical = len(report.Diffs) == 0
	return report
}

func classifyCross(bm, om *model.Marker, mapped alignment.Interval, baseCfg, otherCfg model.Config) CrossMarkerDiff {
	if bm.Code == "sampling-gap" || om.Code == "sampling-gap" {
		return CrossMarkerDiff{Base: bm, Other: om, MappedStartNs: mapped.Start, MappedEndNs: mapped.End, ConfidenceHalfNs: mapped.Half, Cause: CauseSamplingMissing, CauseText: "采样缺段：一方只保留缺失标记，无法把缺失区间补成对方字节", Detail: "缺段与正常字节对应，按采样覆盖问题归类"}
	}
	if sameDecodedEvent(bm, om) {
		return CrossMarkerDiff{Base: bm, Other: om, MappedStartNs: mapped.Start, MappedEndNs: mapped.End, ConfidenceHalfNs: mapped.Half, Cause: CauseUnmatched, CauseText: "时间对应存在残差但解码内容一致", Detail: "保留为对齐残差核对，不归类为信号或协议差异"}
	}
	cause, text, detail := classify(bm, om, snapshot(baseCfg), snapshot(otherCfg))
	if cause == CauseOther && !sameRelevantParameters(baseCfg, otherCfg, bm, om) {
		cause = CauseParameters
		text = "协议参数不同：相同通信事件被按不同波特率、位序、边沿或帧参数解释"
	}
	if cause == CauseBitOrder || cause == CauseClockEdge || cause == CauseIncomplete {
		cause = CauseParameters
		text = "协议参数或片段边界不同：该差异来自解码配置/帧完整性解释"
	}
	if cause == CauseOther {
		cause = CauseSignal
		text = "实际信号不同：两份采样覆盖了同一通信区间，但电平序列或错误状态不一致"
	}
	return CrossMarkerDiff{Base: bm, Other: om, MappedStartNs: mapped.Start, MappedEndNs: mapped.End, ConfidenceHalfNs: mapped.Half, Cause: cause, CauseText: text, Detail: detail}
}

func sameDecodedEvent(a, b *model.Marker) bool {
	if a == nil || b == nil || a.Kind != b.Kind || a.Code != b.Code || a.RawBits != b.RawBits {
		return false
	}
	return (a.Value == nil && b.Value == nil) || (a.Value != nil && b.Value != nil && *a.Value == *b.Value)
}

func sameRelevantParameters(a, b model.Config, bm, om *model.Marker) bool {
	if a.Protocol != b.Protocol {
		return false
	}
	if a.Protocol == "uart" {
		return a.Parameters == b.Parameters
	}
	if a.SPIParameters == nil || b.SPIParameters == nil {
		return a.SPIParameters == b.SPIParameters
	}
	return *a.SPIParameters == *b.SPIParameters
}

func unmatchedOrMissing(m *model.Marker, isBase bool, mapping alignment.Mapping, baseCover, otherCover, mappedGaps []Coverage, chunks []model.Chunk) CrossMarkerDiff {
	iv := alignment.Interval{Start: m.StartNs, End: m.EndNs, Half: uncertaintyFor(m, chunks)}
	if m.Code == "sampling-gap" {
		return CrossMarkerDiff{Base: ternary(isBase, m), Other: ternary(!isBase, m), Cause: CauseSamplingMissing, CauseText: "采样缺段：该事件本身标记为块间隙/局部缺失", Detail: m.Label, MappedStartNs: m.StartNs, MappedEndNs: m.EndNs, ConfidenceHalfNs: iv.Half}
	}
	var target alignment.Interval
	if isBase {
		target = iv
		target.BaseSegmentID = m.SegmentID
	} else {
		var ok bool
		target, ok = mapping.MapInterval(m.SegmentID, m.StartNs, m.EndNs, iv.Half)
		if !ok {
			return CrossMarkerDiff{Other: m, OtherSegmentID: m.SegmentID, OtherChunkID: m.ChunkID, Cause: CauseUnmatched, CauseText: "证据不足：该采集段没有任何锚点，不能跨时钟重置借用另一轮映射", Detail: "保留该段；相同时间戳不代表同一事件", MappedStartNs: m.StartNs, MappedEndNs: m.EndNs, ConfidenceHalfNs: iv.Half}
		}
	}
	if intervalHits(mappedGaps, target) || !intervalHits(baseCoverIf(isBase, baseCover, otherCover), target) {
		return CrossMarkerDiff{Base: ternary(isBase, m), Other: ternary(!isBase, m), Cause: CauseSamplingMissing, CauseText: "采样缺段：另一份采样在可信时间范围内没有覆盖该事件", Detail: "该差异不归因于协议或信号，不补造缺失波形", MappedStartNs: target.Start, MappedEndNs: target.End, ConfidenceHalfNs: target.Half, BaseSegmentID: target.BaseSegmentID, OtherSegmentID: target.OtherSegmentID, AnchorIDs: target.AnchorIDs}
	}
	return CrossMarkerDiff{Base: ternary(isBase, m), Other: ternary(!isBase, m), Cause: CauseUnmatched, CauseText: "证据不足：未找到可唯一对应的共同事件", Detail: "保留无法对应的片段，不强行拉伸成一致波形", MappedStartNs: target.Start, MappedEndNs: target.End, ConfidenceHalfNs: target.Half, BaseSegmentID: target.BaseSegmentID, OtherSegmentID: target.OtherSegmentID, AnchorIDs: target.AnchorIDs}
}

func withNativeRange(diff CrossMarkerDiff, mapping alignment.Mapping) CrossMarkerDiff {
	if diff.Other != nil {
		diff.OtherSegmentID = diff.Other.SegmentID
		diff.OtherChunkID = diff.Other.ChunkID
	}
	if diff.Base == nil {
		return diff
	}
	native, ok := mapping.MapBaseInterval(diff.Base.SegmentID, diff.Base.StartNs, diff.Base.EndNs, diff.ConfidenceHalfNs)
	if ok {
		diff.OtherNativeStartNs = native.Start
		diff.OtherNativeEndNs = native.End
		diff.BaseSegmentID = diff.Base.SegmentID
		diff.OtherSegmentID = native.OtherSegmentID
		diff.BaseChunkID = diff.Base.ChunkID
	}
	if diff.Other != nil {
		diff.OtherSegmentID = diff.Other.SegmentID
		diff.OtherChunkID = diff.Other.ChunkID
	}
	return diff
}

func anchorEvidence(align model.Alignment, baseMarkerID, otherMarkerID string) []string {
	for _, a := range align.Anchors {
		if a.BaseMarkerID == baseMarkerID && a.OtherMarkerID == otherMarkerID {
			return []string{a.ID}
		}
	}
	return nil
}

func ternary(ok bool, m *model.Marker) *model.Marker {
	if ok {
		return m
	}
	return nil
}
func baseCoverIf(isBase bool, base, other []Coverage) []Coverage {
	if isBase {
		return other
	}
	return base
}
func usedByBase(id string, markers []model.Marker) bool {
	for _, m := range markers {
		if m.ID == id {
			return true
		}
	}
	return false
}
func markerIndex(markers []model.Marker, id string) int {
	for i := range markers {
		if markers[i].ID == id {
			return i
		}
	}
	return -1
}
func diffSortKey(d CrossMarkerDiff) int64 {
	if d.Base != nil {
		return d.Base.StartNs
	}
	return d.MappedStartNs
}

func cover(chunks []model.Chunk) []Coverage {
	out := []Coverage{}
	for _, c := range chunks {
		if len(c.Levels) == 0 {
			continue
		}
		out = append(out, Coverage{StartNs: c.StartTime, EndNs: c.EndTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartNs < out[j].StartNs })
	return mergeCover(out)
}

func mappedCover(chunks []model.Chunk, mapping alignment.Mapping) []Coverage {
	out := []Coverage{}
	for _, c := range chunks {
		if len(c.Levels) == 0 {
			continue
		}
		iv, _ := mapping.MapInterval(c.SegmentID, c.StartTime, c.EndTime(), c.IntervalNs/2+1)
		out = append(out, Coverage{StartNs: iv.Start, EndNs: iv.End})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartNs < out[j].StartNs })
	return mergeCover(out)
}

func coverGaps(result model.Result) []Coverage {
	out := []Coverage{}
	for _, m := range result.Markers {
		if m.Code == "sampling-gap" {
			out = append(out, Coverage{StartNs: m.StartNs, EndNs: m.EndNs})
		}
	}
	return mergeCover(out)
}

func mappedGapCoverage(result model.Result, mapping alignment.Mapping) []Coverage {
	out := []Coverage{}
	for _, m := range result.Markers {
		if m.Code == "sampling-gap" {
			iv, _ := mapping.MapInterval(m.SegmentID, m.StartNs, m.EndNs, 1)
			out = append(out, Coverage{StartNs: iv.Start, EndNs: iv.End})
		}
	}
	return mergeCover(out)
}

func mergeCover(in []Coverage) []Coverage {
	if len(in) == 0 {
		return in
	}
	out := []Coverage{in[0]}
	for _, c := range in[1:] {
		last := &out[len(out)-1]
		if c.StartNs <= last.EndNs {
			if c.EndNs > last.EndNs {
				last.EndNs = c.EndNs
			}
			continue
		}
		out = append(out, c)
	}
	return out
}

func intervalHits(cover []Coverage, iv alignment.Interval) bool {
	for _, c := range cover {
		if iv.End+iv.Half >= c.StartNs && iv.Start-iv.Half <= c.EndNs {
			return true
		}
	}
	return false
}

func uncertaintyFor(m *model.Marker, chunks []model.Chunk) int64 {
	maxInterval := int64(1)
	for _, c := range chunks {
		if c.IntervalNs > maxInterval {
			maxInterval = c.IntervalNs
		}
	}
	return int64(math.Max(float64(maxInterval/2+1), float64((m.EndNs-m.StartNs)/20+1)))
}
