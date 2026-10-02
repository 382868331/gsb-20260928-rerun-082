// Package alignment 根据用户确认的共同事件建立跨采样时间映射。
// 映射只在锚点构成的证据区间内使用；证据不足时显式保留无法对应的片段。
package alignment

import (
	"fmt"
	"math"
	"sort"
	"time"

	"logicstation/internal/model"
)

const (
	StatusReady        = "ready"
	StatusInsufficient = "insufficient-evidence"
	CandidateAccepted  = "accepted"
	CandidateExcluded  = "excluded"
)

type Segment struct {
	StartOtherNs int64   `json:"startOtherNs"`
	EndOtherNs   int64   `json:"endOtherNs"`
	OffsetNs     int64   `json:"offsetNs"`
	DriftRate    float64 `json:"driftRate"`
}

type Mapping struct {
	Segments []Segment
}

type Interval struct {
	Start int64
	End   int64
	Half  int64
}

func (i Interval) Overlaps(o Interval) bool {
	return i.Start-i.Half <= o.End+o.Half && i.End+i.Half >= o.Start-o.Half
}

// Build 校验共同事件并生成分段线性映射。锚点按另一采样时间排序，
// 区间外只采用最近锚点的斜率外推，并把外推不确定性放大，避免把漂移变化强行拉直。
func Build(in model.Alignment, baseChunks, otherChunks []model.Chunk) (model.Alignment, Mapping, error) {
	out := in
	if out.ID == "" {
		out.ID = fmt.Sprintf("align-%d", time.Now().UnixNano())
	}
	if out.CreatedAt.IsZero() {
		out.CreatedAt = time.Now().UTC()
	}
	if len(out.Anchors) == 0 {
		out.Status = StatusInsufficient
		return out, Mapping{}, nil
	}
	for i := range out.Anchors {
		if out.Anchors[i].ID == "" {
			out.Anchors[i].ID = fmt.Sprintf("anchor-%d", i+1)
		}
		if out.Anchors[i].UncertaintyNs <= 0 {
			out.Anchors[i].UncertaintyNs = defaultUncertainty(baseChunks, otherChunks)
		}
	}
	sort.SliceStable(out.Anchors, func(i, j int) bool { return out.Anchors[i].OtherTimeNs < out.Anchors[j].OtherTimeNs })
	if repeatedTimes(out.Anchors) {
		return model.Alignment{}, Mapping{}, fmt.Errorf("alignment anchors must use distinct event times on each capture")
	}

	mapping := Mapping{Segments: []Segment{}}
	for i := 0; i+1 < len(out.Anchors); i++ {
		m := segment(out.Anchors[i], out.Anchors[i+1])
		m.StartOtherNs = out.Anchors[i].OtherTimeNs
		m.EndOtherNs = out.Anchors[i+1].OtherTimeNs
		mapping.Segments = append(mapping.Segments, m)
	}
	if len(mapping.Segments) == 0 {
		a := out.Anchors[0]
		mapping.Segments = append(mapping.Segments, Segment{StartOtherNs: math.MinInt64, EndOtherNs: math.MaxInt64, OffsetNs: a.BaseTimeNs - a.OtherTimeNs, DriftRate: 1})
	}
	out.Status = StatusReady
	return out, mapping, nil
}

func repeatedTimes(anchors []model.AlignmentAnchor) bool {
	for i := 1; i < len(anchors); i++ {
		if anchors[i].OtherTimeNs == anchors[i-1].OtherTimeNs {
			return true
		}
	}
	base := append([]model.AlignmentAnchor(nil), anchors...)
	sort.SliceStable(base, func(i, j int) bool { return base[i].BaseTimeNs < base[j].BaseTimeNs })
	for i := 1; i < len(base); i++ {
		if base[i].BaseTimeNs == base[i-1].BaseTimeNs {
			return true
		}
	}
	return false
}

func defaultUncertainty(baseChunks, otherChunks []model.Chunk) int64 {
	maxInterval := int64(1)
	for _, chunks := range [][]model.Chunk{baseChunks, otherChunks} {
		for _, c := range chunks {
			if c.IntervalNs > maxInterval {
				maxInterval = c.IntervalNs
			}
		}
	}
	return maxInterval/2 + 1
}

func segment(a, b model.AlignmentAnchor) Segment {
	dt := float64(b.OtherTimeNs - a.OtherTimeNs)
	rate := float64(b.BaseTimeNs-a.BaseTimeNs) / dt
	return Segment{DriftRate: rate, OffsetNs: a.BaseTimeNs - int64(math.Round(rate*float64(a.OtherTimeNs)))}
}

func (m Mapping) segmentAt(t int64) Segment {
	for _, s := range m.Segments {
		if t >= s.StartOtherNs && t <= s.EndOtherNs {
			return s
		}
	}
	if t < m.Segments[0].StartOtherNs {
		return m.Segments[0]
	}
	return m.Segments[len(m.Segments)-1]
}

// MapInterval 将另一采样的时间中心和波形宽度映射到基准时间轴。
func (m Mapping) MapInterval(start, end int64, uncertainty int64) (Interval, bool) {
	if len(m.Segments) == 0 {
		return Interval{}, false
	}
	c0 := (start + end) / 2
	s := m.segmentAt(c0)
	width := float64(max(end-start, 0)) * math.Abs(s.DriftRate)
	predStart := s.OffsetNs + int64(math.Round(s.DriftRate*float64(start))) - int64(math.Round(width/2))
	predEnd := s.OffsetNs + int64(math.Round(s.DriftRate*float64(end))) + int64(math.Round(width/2))
	half := int64(math.Round(math.Abs(s.DriftRate) * float64(uncertainty)))
	if c0 < s.StartOtherNs || c0 > s.EndOtherNs {
		extrapolation := int64(math.Round(math.Abs(float64(c0-clamp(c0, s.StartOtherNs, s.EndOtherNs)))*math.Abs(s.DriftRate)*0.02)) + 1
		half += extrapolation
	}
	return Interval{Start: predStart, End: predEnd, Half: half}, true
}

// MapBaseInterval 将基准时间轴上的可信区间反算回另一采样的原生时间轴。
func (m Mapping) MapBaseInterval(start, end int64, uncertainty int64) (Interval, bool) {
	if len(m.Segments) == 0 {
		return Interval{}, false
	}
	c0 := (start + end) / 2
	seg := m.segmentAtBase(c0)
	if seg.DriftRate == 0 {
		return Interval{}, false
	}
	width := float64(max(end-start, 0)) / math.Abs(seg.DriftRate)
	startOther := int64(math.Round((float64(start-seg.OffsetNs) - width/2) / seg.DriftRate))
	endOther := int64(math.Round((float64(end-seg.OffsetNs) + width/2) / seg.DriftRate))
	if startOther > endOther {
		startOther, endOther = endOther, startOther
	}
	half := int64(math.Round(float64(uncertainty) / math.Abs(seg.DriftRate)))
	return Interval{Start: startOther, End: endOther, Half: half}, true
}

func (m Mapping) segmentAtBase(t int64) Segment {
	for _, s := range m.Segments {
		start := s.OffsetNs + int64(math.Round(s.DriftRate*float64(s.StartOtherNs)))
		end := s.OffsetNs + int64(math.Round(s.DriftRate*float64(s.EndOtherNs)))
		if start > end {
			start, end = end, start
		}
		if t >= start && t <= end {
			return s
		}
	}
	if len(m.Segments) == 1 {
		return m.Segments[0]
	}
	first := m.Segments[0]
	firstBase := first.OffsetNs + int64(math.Round(first.DriftRate*float64(first.StartOtherNs)))
	if t < firstBase {
		return first
	}
	return m.Segments[len(m.Segments)-1]
}

func clamp(v, lo, hi int64) int64 { return max(lo, min(v, hi)) }
