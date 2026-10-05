package alignment

import (
	"errors"
	"strings"
	"testing"

	"logicstation/internal/model"
)

func chunk(id string, index int, start, interval int64, samples int) model.Chunk {
	return model.Chunk{ID: id, Index: index, StartTime: start, IntervalNs: interval, Levels: make([][]uint8, samples)}
}

func TestSegmentChunksSplitsOnClockReset(t *testing.T) {
	chunks := []model.Chunk{
		chunk("a", 0, 0, 100, 11),    // 段 0：0..1000
		chunk("b", 1, 2000, 100, 11), // 前跳间隙，仍属段 0
		chunk("c", 2, 50, 100, 11),   // 时钟重置回绕，开启段 1
		chunk("d", 3, 1200, 100, 11), // 段 1 内继续
		chunk("e", 4, 10, 100, 11),   // 第二次重置，开启段 2
	}
	segments := model.SegmentChunks(chunks)
	if len(segments) != 3 {
		t.Fatalf("segments=%d, want 3: %+v", len(segments), segments)
	}
	if segments[0].StartNs != 0 || segments[0].EndNs != 3000 {
		t.Fatalf("segment0=%+v", segments[0])
	}
	if segments[1].StartNs != 50 || segments[2].StartNs != 10 {
		t.Fatalf("reset segments=%+v", segments)
	}
}

func resetChunks() (base, other []model.Chunk) {
	base = []model.Chunk{
		chunk("b0", 0, 0, 100, 1001), // 段 0：0..100000
		chunk("b1", 1, 0, 100, 1001), // 重置，段 1：0..100000
		chunk("b2", 2, 0, 100, 1001), // 再次重置，段 2：0..100000
	}
	other = []model.Chunk{chunk("o0", 0, 0, 100, 9001)} // 单段：0..900000
	return base, other
}

func anchor(id string, baseSeg, otherSeg int, baseTime, otherTime int64) model.AlignmentAnchor {
	return model.AlignmentAnchor{
		ID: id, BaseMarkerID: id + "-b", OtherMarkerID: id + "-o",
		BaseTimeNs: baseTime, OtherTimeNs: otherTime, UncertaintyNs: 10,
		BaseSegmentIndex: baseSeg, OtherSegmentIndex: otherSeg,
	}
}

func TestBuildCreatesIndependentMappingPerCaptureSegment(t *testing.T) {
	base, other := resetChunks()
	align := model.Alignment{Anchors: []model.AlignmentAnchor{
		anchor("a1", 0, 0, 10000, 50000),
		anchor("a2", 1, 0, 20000, 350000),
		anchor("a3", 2, 0, 15000, 700000),
	}}
	built, mapping, err := Build(align, base, other)
	if err != nil {
		t.Fatal(err)
	}
	if built.Status != StatusReady || len(mapping.Pairs) != 3 {
		t.Fatalf("status=%s pairs=%d", built.Status, len(mapping.Pairs))
	}
	// 三个采集段上的相同原生时间戳（如 20000ns）必须映射到另一采样的
	// 三个不同轮次，而不是被统一拉伸或落入同一区间。
	rounds := map[int64]int{}
	for segment := 0; segment < 3; segment++ {
		iv, ok := mapping.MapBaseInterval(segment, 20000, 20000, 10)
		if !ok {
			t.Fatalf("segment %d has no mapping", segment)
		}
		rounds[iv.Start]++
	}
	if len(rounds) != 3 {
		t.Fatalf("same timestamp across segments must map to distinct rounds: %v", rounds)
	}
	// 段 2 的 20000ns 应落在第三轮（约 705000），不能进入第一/二轮同名区间。
	iv, _ := mapping.MapBaseInterval(2, 20000, 20000, 10)
	if iv.Start < 600000 {
		t.Fatalf("segment 2 jump landed in wrong round: %+v", iv)
	}
}

func TestBuildRejectsContradictoryOrderingWithinSegmentPair(t *testing.T) {
	base, other := resetChunks()
	align := model.Alignment{Anchors: []model.AlignmentAnchor{
		anchor("a1", 1, 0, 10000, 50000),
		anchor("a2", 1, 0, 5000, 90000), // 另一采样时间更晚，基准时间却更早：倒序矛盾
	}}
	_, _, err := Build(align, base, other)
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("err=%v, want ConflictError", err)
	}
	if len(conflictErr.Conflicts) != 1 {
		t.Fatalf("conflicts=%+v", conflictErr.Conflicts)
	}
	conflict := conflictErr.Conflicts[0]
	if conflict.BaseSegmentIndex != 1 || conflict.OtherSegmentIndex != 0 {
		t.Fatalf("conflict must name the segments: %+v", conflict)
	}
	if !strings.Contains(conflict.Message, "a1") || !strings.Contains(conflict.Message, "a2") {
		t.Fatalf("conflict must name the anchors: %s", conflict.Message)
	}
}

func TestSegmentWithoutAnchorsStaysUnmapped(t *testing.T) {
	base, other := resetChunks()
	align := model.Alignment{Anchors: []model.AlignmentAnchor{
		anchor("a1", 0, 0, 10000, 50000),
	}}
	_, mapping, err := Build(align, base, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mapping.MapBaseInterval(2, 20000, 20000, 10); ok {
		t.Fatal("segment 2 has no anchor evidence and must stay unmapped")
	}
	if _, ok := mapping.MapInterval(0, 50000, 50000, 10); !ok {
		t.Fatal("segment 0 must map through its anchor")
	}
}

// 人工确认跨采集段的先后关系后，首尾逆序（较早段事件对到另一采样末尾、
// 末段事件对到其开头）必须被拒绝，并指出冲突经过的锚点与段。
func TestBuildRejectsReversedOrderAcrossSegments(t *testing.T) {
	base, other := resetChunks()
	align := model.Alignment{Anchors: []model.AlignmentAnchor{
		anchor("first", 0, 0, 10000, 700000), // 第一段事件对到单段另一采样的末尾
		anchor("last", 2, 0, 20000, 50000),   // 末段事件对到其开头：首尾逆序
	}}
	_, _, err := Build(align, base, other)
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("err=%v, want ConflictError", err)
	}
	if len(conflictErr.Conflicts) != 1 {
		t.Fatalf("conflicts=%+v", conflictErr.Conflicts)
	}
	conflict := conflictErr.Conflicts[0]
	if conflict.Kind != "cross-segment-ordering" {
		t.Fatalf("kind=%q, want cross-segment-ordering", conflict.Kind)
	}
	if !strings.Contains(conflict.Message, "first") || !strings.Contains(conflict.Message, "last") {
		t.Fatalf("conflict must name both anchors: %s", conflict.Message)
	}
	if !strings.Contains(conflict.Message, "段 1") || !strings.Contains(conflict.Message, "段 3") {
		t.Fatalf("conflict must name traversed segments: %s", conflict.Message)
	}
}

func TestConsistentWithAnchorOrderKeepsEvidencedSideOnly(t *testing.T) {
	anchors := []model.AlignmentAnchor{anchor("a1", 1, 0, 10000, 300000)}
	// 同一另一轮次（同段同时间一侧无方向证据）不据此排除：保留歧义。
	if !ConsistentWithAnchorOrder(anchors, 0, 10000, 0, 300000) {
		t.Fatal("same other time has no ordering evidence and must stay ambiguous")
	}
	// 更早基准段却对到更晚另一时间：跨段逆序，排除。
	if ConsistentWithAnchorOrder(anchors, 0, 10000, 0, 400000) {
		t.Fatal("reversed cross-segment order must be rejected")
	}
	// 更早基准段对更早另一时间：一致。
	if !ConsistentWithAnchorOrder(anchors, 0, 10000, 0, 200000) {
		t.Fatal("consistent ordering must pass")
	}
}
