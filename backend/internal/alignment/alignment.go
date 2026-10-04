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
	// 该分段映射作用的采集段对：只在此段对内有效，绝不跨段拉伸。
	BaseSegmentIndex  int `json:"baseSegmentIndex"`
	OtherSegmentIndex int `json:"otherSegmentIndex"`
}

// PairMapping 是一对采集段（基准段 ↔ 另一采样段）之间的分段线性映射。
type PairMapping struct {
	BaseSegmentIndex  int
	OtherSegmentIndex int
	AnchorIDs         []string
	Anchors           []model.AlignmentAnchor
	Segments          []Segment
	// DomainStartOtherNs/DomainEndOtherNs 是基准采集段覆盖范围经本段对
	// 反算到另一采样时间轴的定义域：同一另一采样段被多个基准段共用时，
	// 只有落在本段定义域内的时间才由本段对映射，跨段跳转不会进入
	// 错误的一轮同名时间区间。
	DomainStartOtherNs int64
	DomainEndOtherNs   int64
}

// Mapping 由若干段对映射组成；没有锚点证据的段对不参与映射。
type Mapping struct {
	Pairs         []PairMapping
	BaseSegments  []model.CaptureSegment
	OtherSegments []model.CaptureSegment
}

// FlatSegments 展平所有段对的分段，供报告展示。
func (m Mapping) FlatSegments() []Segment {
	out := []Segment{}
	for _, pair := range m.Pairs {
		out = append(out, pair.Segments...)
	}
	return out
}

// AnchorIDsForBaseSegment 返回覆盖该基准段的段对所采用的锚点证据。
func (m Mapping) AnchorIDsForBaseSegment(seg int) []string {
	for _, pair := range m.Pairs {
		if pair.BaseSegmentIndex == seg {
			return pair.AnchorIDs
		}
	}
	return nil
}

// AnchorIDsForOtherSegment 返回覆盖该另一采样段的段对所采用的锚点证据。
func (m Mapping) AnchorIDsForOtherSegment(seg int) []string {
	for _, pair := range m.Pairs {
		if pair.OtherSegmentIndex == seg {
			return pair.AnchorIDs
		}
	}
	return nil
}

// ConflictError 表示人工确认的锚点在同一采集段对内自相矛盾
// （共同事件的先后关系不可能倒序）。错误会指出冲突经过的锚点和段。
type ConflictError struct {
	Conflicts []model.AlignmentConflict
}

func (e *ConflictError) Error() string {
	if len(e.Conflicts) == 0 {
		return "alignment anchors conflict"
	}
	return e.Conflicts[0].Message
}

type Interval struct {
	Start int64
	End   int64
	Half  int64
}

func (i Interval) Overlaps(o Interval) bool {
	return i.Start-i.Half <= o.End+o.Half && i.End+i.Half >= o.Start-o.Half
}

// Build 校验共同事件并按采集段对生成分段线性映射。同一对采集段内锚点按
// 另一采样时间排序，区间外只采用最近锚点的斜率外推并放大不确定性；
// 不同采集段（时钟重置分开的段）各自独立成映射，绝不把整份记录统一拉伸。
// 同一采集段对内锚点顺序自相矛盾时返回 *ConflictError，指出冲突锚点和段。
func Build(in model.Alignment, baseChunks, otherChunks []model.Chunk) (model.Alignment, Mapping, error) {
	out := in
	if out.ID == "" {
		out.ID = fmt.Sprintf("align-%d", time.Now().UnixNano())
	}
	if out.CreatedAt.IsZero() {
		out.CreatedAt = time.Now().UTC()
	}
	mapping := Mapping{
		Pairs:         []PairMapping{},
		BaseSegments:  model.SegmentChunks(baseChunks),
		OtherSegments: model.SegmentChunks(otherChunks),
	}
	if len(out.Anchors) == 0 {
		out.Status = StatusInsufficient
		return out, mapping, nil
	}
	for i := range out.Anchors {
		if out.Anchors[i].ID == "" {
			out.Anchors[i].ID = fmt.Sprintf("anchor-%d", i+1)
		}
		if out.Anchors[i].UncertaintyNs <= 0 {
			out.Anchors[i].UncertaintyNs = defaultUncertainty(baseChunks, otherChunks)
		}
	}
	if conflicts := orderingConflicts(out.Anchors); len(conflicts) > 0 {
		return model.Alignment{}, Mapping{}, &ConflictError{Conflicts: conflicts}
	}

	type pairKey struct{ base, other int }
	groups := map[pairKey][]model.AlignmentAnchor{}
	order := []pairKey{}
	for _, a := range out.Anchors {
		key := pairKey{a.BaseSegmentIndex, a.OtherSegmentIndex}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], a)
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].base != order[j].base {
			return order[i].base < order[j].base
		}
		return order[i].other < order[j].other
	})
	for _, key := range order {
		anchors := groups[key]
		sort.SliceStable(anchors, func(i, j int) bool { return anchors[i].OtherTimeNs < anchors[j].OtherTimeNs })
		pair := PairMapping{BaseSegmentIndex: key.base, OtherSegmentIndex: key.other, Anchors: anchors, Segments: []Segment{}}
		for _, a := range anchors {
			pair.AnchorIDs = append(pair.AnchorIDs, a.ID)
		}
		for i := 0; i+1 < len(anchors); i++ {
			m := segment(anchors[i], anchors[i+1])
			m.StartOtherNs = anchors[i].OtherTimeNs
			m.EndOtherNs = anchors[i+1].OtherTimeNs
			m.BaseSegmentIndex = key.base
			m.OtherSegmentIndex = key.other
			pair.Segments = append(pair.Segments, m)
		}
		if len(pair.Segments) == 0 {
			a := anchors[0]
			pair.Segments = append(pair.Segments, Segment{
				StartOtherNs: math.MinInt64, EndOtherNs: math.MaxInt64,
				OffsetNs: a.BaseTimeNs - a.OtherTimeNs, DriftRate: 1,
				BaseSegmentIndex: key.base, OtherSegmentIndex: key.other,
			})
		}
		pair.DomainStartOtherNs, pair.DomainEndOtherNs = pair.otherDomain(mapping.BaseSegments)
		mapping.Pairs = append(mapping.Pairs, pair)
	}
	out.Status = StatusReady
	return out, mapping, nil
}

// otherDomain 把基准采集段的覆盖范围经本段对反算到另一采样时间轴，
// 作为本段对的定义域；找不到基准段覆盖时退化为锚点区间。
func (p PairMapping) otherDomain(baseSegments []model.CaptureSegment) (int64, int64) {
	var coverage *model.CaptureSegment
	for i := range baseSegments {
		if baseSegments[i].Index == p.BaseSegmentIndex {
			coverage = &baseSegments[i]
			break
		}
	}
	if coverage == nil {
		lo, hi := int64(math.MaxInt64), int64(math.MinInt64)
		for _, a := range p.Anchors {
			lo = min(lo, a.OtherTimeNs)
			hi = max(hi, a.OtherTimeNs)
		}
		return lo, hi
	}
	lo, hi := int64(math.MaxInt64), int64(math.MinInt64)
	for _, s := range p.Segments {
		if s.DriftRate == 0 {
			continue
		}
		for _, baseBound := range [2]int64{coverage.StartNs, coverage.EndNs} {
			other := int64(math.Round(float64(baseBound-s.OffsetNs) / s.DriftRate))
			lo = min(lo, other)
			hi = max(hi, other)
		}
	}
	if lo > hi {
		return p.Anchors[0].OtherTimeNs, p.Anchors[0].OtherTimeNs
	}
	return lo, hi
}

// orderingConflicts 检查同一采集段对内的共同事件先后关系：另一采样时间
// 递增时基准时间也必须严格递增，否则说明把重复报文的不同轮次连错了，
// 或把重置前后的相同时间戳当成了同一事件。返回的冲突指明锚点和段。
func orderingConflicts(anchors []model.AlignmentAnchor) []model.AlignmentConflict {
	type pairKey struct{ base, other int }
	groups := map[pairKey][]model.AlignmentAnchor{}
	for _, a := range anchors {
		key := pairKey{a.BaseSegmentIndex, a.OtherSegmentIndex}
		groups[key] = append(groups[key], a)
	}
	conflicts := []model.AlignmentConflict{}
	for key, group := range groups {
		sort.SliceStable(group, func(i, j int) bool { return group[i].OtherTimeNs < group[j].OtherTimeNs })
		for i := 1; i < len(group); i++ {
			prev, cur := group[i-1], group[i]
			if cur.OtherTimeNs == prev.OtherTimeNs || cur.BaseTimeNs <= prev.BaseTimeNs {
				conflicts = append(conflicts, model.AlignmentConflict{
					Kind:              "anchor-ordering",
					Message:           fmt.Sprintf("锚点 %s 与 %s 在采集段对（基准段 %d ↔ 另一段 %d）内先后关系矛盾：共同事件不可能倒序，请确认是否把重复报文的不同轮次连错", prev.ID, cur.ID, key.base, key.other),
					PreviousAnchorID:  prev.ID,
					NewAnchorID:       cur.ID,
					BaseSegmentIndex:  key.base,
					OtherSegmentIndex: key.other,
				})
			}
		}
	}
	return conflicts
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

// PairForOther 找到覆盖另一采样某段的段对映射。同一另一采样段被多个
// 基准段共用（对方没有重置而本方有）时，按各段对定义域（基准段覆盖
// 反算而来）选择，保证落在正确的一轮；没有该段的锚点证据时返回 false。
func (m Mapping) PairForOther(otherSegment int, t int64) (PairMapping, bool) {
	var best *PairMapping
	bestDistance := int64(math.MaxInt64)
	for i := range m.Pairs {
		pair := &m.Pairs[i]
		if pair.OtherSegmentIndex != otherSegment {
			continue
		}
		distance := int64(0)
		if t < pair.DomainStartOtherNs {
			distance = pair.DomainStartOtherNs - t
		} else if t > pair.DomainEndOtherNs {
			distance = t - pair.DomainEndOtherNs
		}
		if distance < bestDistance {
			best, bestDistance = pair, distance
		}
	}
	if best == nil {
		return PairMapping{}, false
	}
	return *best, true
}

// PairForBase 按基准段和基准时间反查段对，保证跨段跳转落在锚点证据
// 所指的那一轮同名时间区间，而不是另一轮重复报文。
func (m Mapping) PairForBase(baseSegment int, baseTime int64) (PairMapping, bool) {
	var best *PairMapping
	bestDistance := int64(math.MaxInt64)
	for i := range m.Pairs {
		pair := &m.Pairs[i]
		if pair.BaseSegmentIndex != baseSegment {
			continue
		}
		distance := int64(math.MaxInt64)
		for _, a := range pair.Anchors {
			if d := abs64(baseTime - a.BaseTimeNs); d < distance {
				distance = d
			}
		}
		if distance < bestDistance {
			best, bestDistance = pair, distance
		}
	}
	if best == nil {
		return PairMapping{}, false
	}
	return *best, true
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (p PairMapping) segmentAt(t int64) Segment {
	for _, s := range p.Segments {
		if t >= s.StartOtherNs && t <= s.EndOtherNs {
			return s
		}
	}
	if t < p.Segments[0].StartOtherNs {
		return p.Segments[0]
	}
	return p.Segments[len(p.Segments)-1]
}

// MapInterval 将另一采样指定采集段的时间中心和波形宽度映射到基准时间轴。
// 段号来自解码标记的 SegmentIndex，重置前后相同时间戳因此走不同段对。
func (m Mapping) MapInterval(otherSegment int, start, end int64, uncertainty int64) (Interval, bool) {
	c0 := (start + end) / 2
	pair, ok := m.PairForOther(otherSegment, c0)
	if !ok {
		return Interval{}, false
	}
	return pair.MapInterval(start, end, uncertainty)
}

// MapInterval 在该段对内把另一采样时间映射到基准时间轴。
func (p PairMapping) MapInterval(start, end int64, uncertainty int64) (Interval, bool) {
	c0 := (start + end) / 2
	s := p.segmentAt(c0)
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

// MapBaseInterval 将基准采集段上的可信区间反算回另一采样对应段的原生时间轴。
func (m Mapping) MapBaseInterval(baseSegment int, start, end int64, uncertainty int64) (Interval, bool) {
	c0 := (start + end) / 2
	pair, ok := m.PairForBase(baseSegment, c0)
	if !ok {
		return Interval{}, false
	}
	return pair.MapBaseInterval(start, end, uncertainty)
}

// MapBaseInterval 在该段对内把基准时间反算回另一采样的原生时间轴。
func (p PairMapping) MapBaseInterval(start, end int64, uncertainty int64) (Interval, bool) {
	c0 := (start + end) / 2
	seg := p.segmentAtBase(c0)
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

func (p PairMapping) segmentAtBase(t int64) Segment {
	for _, s := range p.Segments {
		start := s.OffsetNs + int64(math.Round(s.DriftRate*float64(s.StartOtherNs)))
		end := s.OffsetNs + int64(math.Round(s.DriftRate*float64(s.EndOtherNs)))
		if start > end {
			start, end = end, start
		}
		if t >= start && t <= end {
			return s
		}
	}
	if len(p.Segments) == 1 {
		return p.Segments[0]
	}
	first := p.Segments[0]
	firstBase := first.OffsetNs + int64(math.Round(first.DriftRate*float64(first.StartOtherNs)))
	if t < firstBase {
		return first
	}
	return p.Segments[len(p.Segments)-1]
}

func clamp(v, lo, hi int64) int64 { return max(lo, min(v, hi)) }
