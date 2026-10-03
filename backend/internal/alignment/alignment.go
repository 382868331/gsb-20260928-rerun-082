// Package alignment maps matching events between captures whose local clocks
// may reset. All lookups require a capture segment: identical timestamps in
// different reset rounds are never treated as the same event.
package alignment

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"logicstation/internal/model"
)

const (
	StatusReady        = "ready"
	StatusInsufficient = "insufficient-evidence"
	StatusConflict     = "order-conflict"
	CandidateAccepted  = "accepted"
	CandidateExcluded  = "excluded"
)

type Segment struct {
	ID             string   `json:"id"`
	BaseSegmentID  string   `json:"baseSegmentId"`
	OtherSegmentID string   `json:"otherSegmentId"`
	Sequence       int      `json:"sequence"`
	StartOtherNs   int64    `json:"startOtherNs"`
	EndOtherNs     int64    `json:"endOtherNs"`
	StartBaseNs    int64    `json:"startBaseNs"`
	EndBaseNs      int64    `json:"endBaseNs"`
	OffsetNs       int64    `json:"offsetNs"`
	DriftRate      float64  `json:"driftRate"`
	AnchorIDs      []string `json:"anchorIds,omitempty"`
}

type Mapping struct{ Segments []Segment }

type Interval struct {
	Start          int64
	End            int64
	Half           int64
	BaseSegmentID  string
	OtherSegmentID string
	Extrapolated   bool
	AnchorIDs      []string
}

func (i Interval) Overlaps(o Interval) bool {
	return i.Start-i.Half <= o.End+o.Half && i.End+i.Half >= o.Start-o.Half
}

func Build(in model.Alignment, baseChunks, otherChunks []model.Chunk) (model.Alignment, Mapping, error) {
	model.NormalizeChunkSegments(baseChunks)
	model.NormalizeChunkSegments(otherChunks)
	out := in
	if out.ID == "" {
		out.ID = fmt.Sprintf("align-%d", time.Now().UnixNano())
	}
	if out.CreatedAt.IsZero() {
		out.CreatedAt = time.Now().UTC()
	}
	out.BaseSegments = model.CaptureSegments(out.BaseCaptureID, baseChunks)
	out.OtherSegments = model.CaptureSegments(out.OtherCaptureID, otherChunks)
	if len(out.Anchors) == 0 {
		out.Status = StatusInsufficient
		return out, Mapping{}, nil
	}
	if err := normalizeAndValidateAnchors(&out); err != nil {
		out.Status = StatusConflict
		return out, Mapping{}, err
	}

	groups := map[string][]model.AlignmentAnchor{}
	for _, anchor := range out.Anchors {
		groups[anchor.OtherSegmentID] = append(groups[anchor.OtherSegmentID], anchor)
	}
	mapping := Mapping{Segments: []Segment{}}
	for otherSegmentID, anchors := range groups {
		sort.SliceStable(anchors, func(i, j int) bool { return anchors[i].Sequence < anchors[j].Sequence })
		baseSegmentID := anchors[0].BaseSegmentID
		for _, anchor := range anchors[1:] {
			if anchor.BaseSegmentID != baseSegmentID {
				return out, Mapping{}, fmt.Errorf("锚点 %s 和 %s 把另一采样段 %s 同时对应到基准段 %s 和 %s", anchors[0].ID, anchor.ID, otherSegmentID, baseSegmentID, anchor.BaseSegmentID)
			}
		}
		for i := 0; i+1 < len(anchors); i++ {
			a, b := anchors[i], anchors[i+1]
			if b.OtherTimeNs <= a.OtherTimeNs || b.BaseTimeNs <= a.BaseTimeNs {
				out.Status = StatusConflict
				return out, Mapping{}, fmt.Errorf("段 %s 内人工确认先后与时间顺序矛盾：锚点 %s -> %s", otherSegmentID, a.ID, b.ID)
			}
			m := localSegment(a, b)
			m.ID = fmt.Sprintf("%s->%s:%s", a.BaseSegmentID, a.OtherSegmentID, a.ID)
			m.BaseSegmentID = baseSegmentID
			m.OtherSegmentID = otherSegmentID
			m.Sequence = a.Sequence
			m.AnchorIDs = []string{a.ID, b.ID}
			mapping.Segments = append(mapping.Segments, m)
		}
		if len(anchors) == 1 {
			a := anchors[0]
			mapping.Segments = append(mapping.Segments, Segment{
				ID:            fmt.Sprintf("%s->%s:%s", a.BaseSegmentID, a.OtherSegmentID, a.ID),
				BaseSegmentID: a.BaseSegmentID, OtherSegmentID: a.OtherSegmentID, Sequence: a.Sequence,
				StartOtherNs: math.MinInt64, EndOtherNs: math.MaxInt64, StartBaseNs: math.MinInt64, EndBaseNs: math.MaxInt64,
				OffsetNs: a.BaseTimeNs - a.OtherTimeNs, DriftRate: 1, AnchorIDs: []string{a.ID},
			})
		}
	}
	sort.SliceStable(mapping.Segments, func(i, j int) bool { return mapping.Segments[i].Sequence < mapping.Segments[j].Sequence })
	out.Status = StatusReady
	return out, mapping, nil
}

func normalizeAndValidateAnchors(out *model.Alignment) error {
	for i := range out.Anchors {
		a := &out.Anchors[i]
		if a.ID == "" {
			a.ID = fmt.Sprintf("anchor-%d", i+1)
		}
		if a.Sequence == 0 {
			a.Sequence = i + 1
		}
		if a.UncertaintyNs <= 0 {
			a.UncertaintyNs = 1
		}
	}
	sort.SliceStable(out.Anchors, func(i, j int) bool { return out.Anchors[i].Sequence < out.Anchors[j].Sequence })
	seen := map[string]bool{}
	for i, a := range out.Anchors {
		key := a.BaseMarkerID + "\x00" + a.OtherMarkerID
		if seen[key] {
			return fmt.Errorf("duplicate anchor %s", a.ID)
		}
		seen[key] = true
		if a.BaseSegmentID == "" {
			if id, ok := segmentContainingTime(out.BaseSegments, a.BaseTimeNs); ok {
				a.BaseSegmentID = id
			}
		}
		if a.OtherSegmentID == "" {
			if id, ok := segmentContainingTime(out.OtherSegments, a.OtherTimeNs); ok {
				a.OtherSegmentID = id
			}
		}
		if a.BaseSegmentID == "" || a.OtherSegmentID == "" {
			return fmt.Errorf("anchor %s is missing segment identity", a.ID)
		}
		if segmentSequence(out.BaseSegments, a.BaseSegmentID) < 0 || segmentSequence(out.OtherSegments, a.OtherSegmentID) < 0 {
			return fmt.Errorf("anchor %s references an unknown capture segment", a.ID)
		}
		for _, b := range out.Anchors[:i] {
			if a.BaseSegmentID == b.BaseSegmentID && a.BaseTimeNs == b.BaseTimeNs {
				return fmt.Errorf("锚点 %s 和 %s 在同一基准段使用相同时间戳", b.ID, a.ID)
			}
			if a.OtherSegmentID == b.OtherSegmentID && a.OtherTimeNs == b.OtherTimeNs {
				return fmt.Errorf("锚点 %s 和 %s 在同一另一采样段使用相同时间戳", b.ID, a.ID)
			}
		}
	}
	for i := 0; i+1 < len(out.Anchors); i++ {
		a, b := out.Anchors[i], out.Anchors[i+1]
		if a.ManualOrderConfirmed && b.ManualOrderConfirmed {
			if segmentSequence(out.BaseSegments, b.BaseSegmentID) < segmentSequence(out.BaseSegments, a.BaseSegmentID) ||
				segmentSequence(out.OtherSegments, b.OtherSegmentID) < segmentSequence(out.OtherSegments, a.OtherSegmentID) {
				return fmt.Errorf("人工确认顺序冲突：锚点 %s(%s/%s) 不能位于 %s(%s/%s) 之后；旧对齐不会被自动改写", a.ID, a.BaseSegmentID, a.OtherSegmentID, b.ID, b.BaseSegmentID, b.OtherSegmentID)
			}
		}
	}
	return nil
}

func segmentContainingTime(segments []model.CaptureSegment, t int64) (string, bool) {
	matches := []string{}
	for _, s := range segments {
		if t >= s.StartNs && t <= s.EndNs {
			matches = append(matches, s.ID)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return "", false
}

func segmentSequence(segments []model.CaptureSegment, id string) int {
	for _, s := range segments {
		if s.ID == id {
			return s.Sequence
		}
	}
	return -1
}

func localSegment(a, b model.AlignmentAnchor) Segment {
	rate := float64(b.BaseTimeNs-a.BaseTimeNs) / float64(b.OtherTimeNs-a.OtherTimeNs)
	return Segment{
		StartOtherNs: a.OtherTimeNs, EndOtherNs: b.OtherTimeNs, StartBaseNs: a.BaseTimeNs, EndBaseNs: b.BaseTimeNs,
		DriftRate: rate, OffsetNs: a.BaseTimeNs - int64(math.Round(rate*float64(a.OtherTimeNs))),
	}
}

func (m Mapping) segmentAt(segmentID string, t int64) (Segment, bool) {
	var fallback *Segment
	for i := range m.Segments {
		s := &m.Segments[i]
		if s.OtherSegmentID != segmentID {
			continue
		}
		if t >= s.StartOtherNs && t <= s.EndOtherNs {
			return *s, true
		}
		if fallback == nil {
			fallback = s
		}
	}
	if fallback != nil {
		return *fallback, true
	}
	return Segment{}, false
}

func (m Mapping) MapInterval(segmentID string, start, end, uncertainty int64) (Interval, bool) {
	if segmentID == "" {
		return Interval{}, false
	}
	c0 := (start + end) / 2
	s, ok := m.segmentAt(segmentID, c0)
	if !ok {
		return Interval{}, false
	}
	width := float64(max(end-start, 0)) * math.Abs(s.DriftRate)
	predStart := s.OffsetNs + int64(math.Round(s.DriftRate*float64(start))) - int64(math.Round(width/2))
	predEnd := s.OffsetNs + int64(math.Round(s.DriftRate*float64(end))) + int64(math.Round(width/2))
	half := int64(math.Round(math.Abs(s.DriftRate)*float64(uncertainty))) + 1
	extrapolated := c0 < s.StartOtherNs || c0 > s.EndOtherNs
	if extrapolated {
		half += int64(math.Round(math.Abs(float64(c0-clamp(c0, s.StartOtherNs, s.EndOtherNs)))*math.Abs(s.DriftRate)*0.02)) + 1
	}
	return Interval{Start: predStart, End: predEnd, Half: half, BaseSegmentID: s.BaseSegmentID, OtherSegmentID: segmentID, Extrapolated: extrapolated, AnchorIDs: s.AnchorIDs}, true
}

func (m Mapping) MapBaseInterval(baseSegmentID string, start, end, uncertainty int64) (Interval, bool) {
	if baseSegmentID == "" {
		return Interval{}, false
	}
	var seg *Segment
	for i := range m.Segments {
		if m.Segments[i].BaseSegmentID == baseSegmentID {
			seg = &m.Segments[i]
			break
		}
	}
	if seg == nil || seg.DriftRate == 0 {
		return Interval{}, false
	}
	width := float64(max(end-start, 0)) / math.Abs(seg.DriftRate)
	startOther := int64(math.Round((float64(start-seg.OffsetNs) - width/2) / seg.DriftRate))
	endOther := int64(math.Round((float64(end-seg.OffsetNs) + width/2) / seg.DriftRate))
	if startOther > endOther {
		startOther, endOther = endOther, startOther
	}
	half := int64(math.Round(float64(uncertainty)/math.Abs(seg.DriftRate))) + 1
	return Interval{Start: startOther, End: endOther, Half: half, BaseSegmentID: baseSegmentID, OtherSegmentID: seg.OtherSegmentID, Extrapolated: start < seg.StartBaseNs || end > seg.EndBaseNs, AnchorIDs: seg.AnchorIDs}, true
}

func (m Mapping) SegmentSummary() string {
	parts := make([]string, 0, len(m.Segments))
	for _, s := range m.Segments {
		parts = append(parts, fmt.Sprintf("%s→%s 偏移 %dns 漂移 %.6f", s.OtherSegmentID, s.BaseSegmentID, s.OffsetNs, s.DriftRate))
	}
	return strings.Join(parts, "；")
}

func clamp(v, lo, hi int64) int64 { return max(lo, min(v, hi)) }
