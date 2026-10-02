// Package align 处理跨采样对齐：根据用户选定的共同事件（锚点）估计两份采样
// 时间轴之间的偏移与漂移，给出每个锚点的残差和整体可信范围；
// 证据不足（锚点覆盖范围之外）的片段保持无法对应，绝不外插拉伸成看似一致的波形。
package align

import (
	"fmt"
	"sort"

	"logicstation/internal/model"
)

// 跨采样差异根因类别。
const (
	CauseSignalDifference = "signal-difference"
	CauseSamplingGap      = "sampling-gap"
	CauseDecodeConfig     = "decode-config"
	CauseUnmapped         = "unmapped"
)

// AnchorOffset 是单个锚点的偏移观测值及其相对拟合直线的残差。
type AnchorOffset struct {
	AnchorID   string `json:"anchorId"`
	Label      string `json:"label,omitempty"`
	BaseNs     int64  `json:"baseNs"`
	OtherNs    int64  `json:"otherNs"`
	OffsetNs   int64  `json:"offsetNs"`
	ResidualNs int64  `json:"residualNs"`
}

// Estimate 是由锚点集合得到的对齐估计。
type Estimate struct {
	AnchorCount     int            `json:"anchorCount"`
	OffsetNs        int64          `json:"offsetNs"`
	MinOffsetNs     int64          `json:"minOffsetNs"`
	MaxOffsetNs     int64          `json:"maxOffsetNs"`
	DriftPpm        float64        `json:"driftPpm"`
	HasDrift        bool           `json:"hasDrift"`
	CoverageStartNs int64          `json:"coverageStartNs"`
	CoverageEndNs   int64          `json:"coverageEndNs"`
	ConfidenceNs    int64          `json:"confidenceNs"`
	Anchors         []AnchorOffset `json:"anchors"`

	scale     float64
	intercept float64
}

// EstimateOffsets 对锚点做最小二乘直线拟合 other = scale*base + intercept。
// 单锚点时退化为纯偏移（无法估计漂移）；零锚点时返回空估计。
func EstimateOffsets(anchors []model.AlignmentAnchor) Estimate {
	est := Estimate{AnchorCount: len(anchors), Anchors: []AnchorOffset{}}
	if len(anchors) == 0 {
		return est
	}
	sorted := append([]model.AlignmentAnchor(nil), anchors...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].BaseNs < sorted[j].BaseNs })

	offsets := make([]int64, 0, len(sorted))
	for _, a := range sorted {
		offsets = append(offsets, a.OtherNs-a.BaseNs)
	}
	est.MinOffsetNs, est.MaxOffsetNs = offsets[0], offsets[0]
	for _, o := range offsets {
		if o < est.MinOffsetNs {
			est.MinOffsetNs = o
		}
		if o > est.MaxOffsetNs {
			est.MaxOffsetNs = o
		}
	}
	est.CoverageStartNs = sorted[0].BaseNs
	est.CoverageEndNs = sorted[len(sorted)-1].BaseNs

	if len(sorted) == 1 {
		est.scale = 1
		est.intercept = float64(offsets[0])
		est.OffsetNs = offsets[0]
		est.Anchors = append(est.Anchors, AnchorOffset{
			AnchorID: sorted[0].ID, Label: sorted[0].Label,
			BaseNs: sorted[0].BaseNs, OtherNs: sorted[0].OtherNs, OffsetNs: offsets[0],
		})
		return est
	}

	var sx, sy, sxx, sxy float64
	n := float64(len(sorted))
	for _, a := range sorted {
		x, y := float64(a.BaseNs), float64(a.OtherNs)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	denom := n*sxx - sx*sx
	if denom == 0 {
		est.scale = 1
		est.intercept = sy/n - sx/n
	} else {
		est.scale = (n*sxy - sx*sy) / denom
		est.intercept = (sy - est.scale*sx) / n
	}
	est.HasDrift = true
	est.DriftPpm = (est.scale - 1) * 1e6
	est.OffsetNs = int64(est.intercept + est.scale*sx/n - sx/n)

	for i, a := range sorted {
		fitted := est.scale*float64(a.BaseNs) + est.intercept
		residual := int64(float64(a.OtherNs) - fitted)
		if residual < 0 {
			residual = -residual
		}
		if residual > est.ConfidenceNs {
			est.ConfidenceNs = residual
		}
		est.Anchors = append(est.Anchors, AnchorOffset{
			AnchorID: a.ID, Label: a.Label,
			BaseNs: a.BaseNs, OtherNs: a.OtherNs,
			OffsetNs:   offsets[i],
			ResidualNs: int64(float64(a.OtherNs) - fitted),
		})
	}
	return est
}

// MappedInterval 是基准采样时间区间在对照采样时间轴上的映射结果。
type MappedInterval struct {
	Mapped       bool   `json:"mapped"`
	Reason       string `json:"reason,omitempty"`
	OtherStartNs int64  `json:"otherStartNs,omitempty"`
	OtherEndNs   int64  `json:"otherEndNs,omitempty"`
	LowNs        int64  `json:"lowNs,omitempty"`
	HighNs       int64  `json:"highNs,omitempty"`
}

// MapInterval 将基准采样的 [startNs,endNs] 映射到对照采样时间轴。
// 区间落在锚点覆盖范围之外时返回无法对应，不做外插。
func (est Estimate) MapInterval(startNs, endNs int64) MappedInterval {
	if est.AnchorCount == 0 {
		return MappedInterval{Mapped: false, Reason: "没有对齐锚点，两份采样无法对应"}
	}
	// 单锚点只有纯偏移、无法估计漂移，整轴按同一偏移映射；
	// 两个以上锚点时，覆盖范围之外的片段证据不足，保持无法对应，不外插。
	if est.AnchorCount > 1 && (startNs < est.CoverageStartNs || endNs > est.CoverageEndNs) {
		return MappedInterval{
			Mapped: false,
			Reason: fmt.Sprintf("区间超出锚点覆盖范围（%d–%d ns），证据不足，保留为无法对应片段", est.CoverageStartNs, est.CoverageEndNs),
		}
	}
	start := int64(est.scale*float64(startNs) + est.intercept)
	end := int64(est.scale*float64(endNs) + est.intercept)
	return MappedInterval{
		Mapped:       true,
		OtherStartNs: start,
		OtherEndNs:   end,
		LowNs:        start - est.ConfidenceNs,
		HighNs:       end + est.ConfidenceNs,
	}
}

// Candidate 是一对自动发现的候选共同事件；被排除的候选保留原因供核对。
type Candidate struct {
	Value         byte   `json:"value"`
	BaseMarkerID  string `json:"baseMarkerId"`
	OtherMarkerID string `json:"otherMarkerId"`
	BaseNs        int64  `json:"baseNs"`
	OtherNs       int64  `json:"otherNs"`
	OffsetNs      int64  `json:"offsetNs"`
	Excluded      bool   `json:"excluded"`
	Reason        string `json:"reason,omitempty"`
}

func byteMarkers(markers []model.Marker) []model.Marker {
	out := []model.Marker{}
	for _, m := range markers {
		if m.Kind == "byte" && m.Value != nil {
			out = append(out, m)
		}
	}
	return out
}

// AnchorCandidates 按字节值顺序配对两份解码结果中的字节标记。
// 同一字节值在对照侧有多个未匹配标记（重复脉冲/重复字节）时无法唯一对应，
// 该候选被排除并记录原因，绝不任意挑选。
func AnchorCandidates(base, other []model.Marker) (accepted, excluded []Candidate) {
	accepted, excluded = []Candidate{}, []Candidate{}
	baseBytes := byteMarkers(base)
	otherBytes := byteMarkers(other)
	used := make([]bool, len(otherBytes))
	for _, bm := range baseBytes {
		var matches []int
		for i, om := range otherBytes {
			if !used[i] && *om.Value == *bm.Value {
				matches = append(matches, i)
			}
		}
		candidate := Candidate{
			Value: *bm.Value, BaseMarkerID: bm.ID, BaseNs: bm.StartNs,
		}
		switch {
		case len(matches) == 1:
			om := otherBytes[matches[0]]
			used[matches[0]] = true
			candidate.OtherMarkerID = om.ID
			candidate.OtherNs = om.StartNs
			candidate.OffsetNs = om.StartNs - bm.StartNs
			accepted = append(accepted, candidate)
		case len(matches) > 1:
			candidate.Excluded = true
			candidate.Reason = fmt.Sprintf("字节 0x%02X 在对照采样中还有 %d 个未匹配重复出现（重复脉冲），无法唯一对应", *bm.Value, len(matches))
			excluded = append(excluded, candidate)
		default:
			candidate.Excluded = true
			candidate.Reason = fmt.Sprintf("字节 0x%02X 在对照采样中没有可配对的标记（可能缺段或信号不同）", *bm.Value)
			excluded = append(excluded, candidate)
		}
	}
	return accepted, excluded
}

// CrossDiff 是跨采样对照中的一处差异或无法对应片段。
type CrossDiff struct {
	Index            int           `json:"index"`
	Base             *model.Marker `json:"base,omitempty"`
	Other            *model.Marker `json:"other,omitempty"`
	BaseStartNs      int64         `json:"baseStartNs"`
	BaseEndNs        int64         `json:"baseEndNs"`
	OtherStartNs     int64         `json:"otherStartNs,omitempty"`
	OtherEndNs       int64         `json:"otherEndNs,omitempty"`
	ConfidenceLowNs  int64         `json:"confidenceLowNs,omitempty"`
	ConfidenceHighNs int64         `json:"confidenceHighNs,omitempty"`
	Cause            string        `json:"cause"`
	CauseText        string        `json:"causeText"`
	Detail           string        `json:"detail"`
}

// CrossReport 是两份采样解码结果在对齐后的对照报告。
type CrossReport struct {
	AlignmentID   string      `json:"alignmentId"`
	Version       int         `json:"version"`
	BaseResultID  string      `json:"baseResultId"`
	OtherResultID string      `json:"otherResultId"`
	Estimate      Estimate    `json:"estimate"`
	Matched       int         `json:"matched"`
	DiffCount     int         `json:"diffCount"`
	Diffs         []CrossDiff `json:"diffs"`
}

func configsDiffer(base, other model.Config) bool {
	if base.Protocol != other.Protocol {
		return true
	}
	if base.Parameters != other.Parameters {
		return true
	}
	if (base.SPIParameters == nil) != (other.SPIParameters == nil) {
		return true
	}
	if base.SPIParameters != nil && other.SPIParameters != nil && *base.SPIParameters != *other.SPIParameters {
		return true
	}
	return false
}

func sameMarker(a, b *model.Marker) bool {
	if a.Kind != b.Kind || a.Code != b.Code || a.RawBits != b.RawBits {
		return false
	}
	if (a.Value == nil) != (b.Value == nil) {
		return false
	}
	return a.Value == nil || *a.Value == *b.Value
}

func overlap(aStart, aEnd, bStart, bEnd int64) int64 {
	start := max(aStart, bStart)
	end := min(aEnd, bEnd)
	if end <= start {
		return 0
	}
	return end - start
}

// CompareResults 在对齐估计下对照两份采样的解码结果，区分实际信号不同、
// 采样缺段和协议参数选择造成的差异；锚点覆盖之外的标记保留为无法对应片段。
func CompareResults(alignment model.Alignment, base, other model.Result, baseCfg, otherCfg model.Config) CrossReport {
	est := EstimateOffsets(alignment.Anchors)
	report := CrossReport{
		AlignmentID:   alignment.ID,
		Version:       alignment.Version,
		BaseResultID:  base.ID,
		OtherResultID: other.ID,
		Estimate:      est,
		Diffs:         []CrossDiff{},
	}
	used := make([]bool, len(other.Markers))
	add := func(diff CrossDiff) {
		diff.Index = len(report.Diffs)
		report.Diffs = append(report.Diffs, diff)
	}
	for i := range base.Markers {
		bm := &base.Markers[i]
		mapped := est.MapInterval(bm.StartNs, bm.EndNs)
		diff := CrossDiff{Base: bm, BaseStartNs: bm.StartNs, BaseEndNs: bm.EndNs}
		if !mapped.Mapped {
			diff.Cause = CauseUnmapped
			diff.CauseText = "无法对应：该片段在锚点覆盖范围之外，证据不足，未强行对齐"
			diff.Detail = mapped.Reason
			add(diff)
			continue
		}
		diff.OtherStartNs = mapped.OtherStartNs
		diff.OtherEndNs = mapped.OtherEndNs
		diff.ConfidenceLowNs = mapped.LowNs
		diff.ConfidenceHighNs = mapped.HighNs
		best, bestOverlap := -1, int64(0)
		for j := range other.Markers {
			if used[j] {
				continue
			}
			om := &other.Markers[j]
			if ov := overlap(mapped.LowNs, mapped.HighNs, om.StartNs, om.EndNs); ov > bestOverlap {
				best, bestOverlap = j, ov
			}
		}
		if best < 0 {
			diff.Cause = CauseSamplingGap
			diff.CauseText = "采样缺段：对照采样在对应区间没有任何解码标记"
			diff.Detail = fmt.Sprintf("映射区间 %d–%d ns（可信范围 ±%d ns）内无标记，疑似对照采样缺失该段", mapped.OtherStartNs, mapped.OtherEndNs, est.ConfidenceNs)
			add(diff)
			continue
		}
		om := &other.Markers[best]
		used[best] = true
		diff.Other = om
		switch {
		case sameMarker(bm, om):
			report.Matched++
			continue
		case bm.Code == "sampling-gap" || om.Code == "sampling-gap":
			diff.Cause = CauseSamplingGap
			diff.CauseText = "采样缺段：一侧在该区间标记了采样间隙"
			diff.Detail = "采样时间轴不连续导致该段无法按正常帧解码，不补造字节"
		case configsDiffer(baseCfg, otherCfg):
			diff.Cause = CauseDecodeConfig
			diff.CauseText = "协议参数不同：两份结果使用的解码配置不一致"
			diff.Detail = fmt.Sprintf("基准 %q vs 对照 %q；先统一配置再判断是否为真实信号差异", bm.Label, om.Label)
		default:
			diff.Cause = CauseSignalDifference
			diff.CauseText = "实际信号不同：相同解码配置下对应区间内容不一致"
			diff.Detail = fmt.Sprintf("基准 %q，对照 %q", bm.Label, om.Label)
		}
		add(diff)
	}
	for j := range other.Markers {
		if used[j] {
			continue
		}
		om := &other.Markers[j]
		add(CrossDiff{
			Other:        om,
			OtherStartNs: om.StartNs,
			OtherEndNs:   om.EndNs,
			Cause:        CauseSignalDifference,
			CauseText:    "实际信号不同：对照采样多出的标记，基准采样对应区间无对应",
			Detail:       fmt.Sprintf("对照侧多出 %q", om.Label),
		})
	}
	report.DiffCount = len(report.Diffs)
	return report
}
