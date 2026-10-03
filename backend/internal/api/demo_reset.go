package api

import (
	"fmt"

	"logicstation/internal/alignment"
	"logicstation/internal/model"
	"logicstation/internal/store"
)

func createClockResetCapture(st *store.Store, first bool) map[string]any {
	nameA, nameB := "时钟重置人工波形 A", "时钟重置人工波形 B"
	if captureA, okA := st.FindCaptureByName(nameA); okA {
		if captureB, okB := st.FindCaptureByName(nameB); okB {
			if first {
				_, chunksA, _ := st.GetCapture(captureA.ID)
				return demoPayload(st, captureA, chunksA)
			}
			_, chunksB, _ := st.GetCapture(captureB.ID)
			return demoPayload(st, captureB, chunksB)
		}
	}
	captureA, ok := st.FindCaptureByName(nameA)
	if !ok {
		captureA = st.CreateCapture(nameA, []string{"CLK-REF", "RX"})
	}
	captureB := st.CreateCapture(nameB, []string{"CLK-REF", "RX"})

	const interval = int64(1000)
	const bit = int64(10000)
	values := []byte{'A', 'B', 'A', 'C'}
	starts := []int64{10000, 130000, 10000, 130000}

	build := func(capture model.Capture, offset int64) []model.Chunk {
		changes := []change{{at: 0, level: 1}}
		for i, value := range values {
			cursor := starts[i]
			changes = append(changes, change{at: cursor, level: 0})
			for bitIndex := 0; bitIndex < 8; bitIndex++ {
				changes = append(changes, change{at: cursor + bit*int64(bitIndex+1), level: (value >> bitIndex) & 1})
			}
			changes = append(changes, change{at: cursor + bit*9, level: 1})
		}
		rows := func(from, to int64) [][]uint8 {
			out := [][]uint8{}
			for t := from; t <= to; t += interval {
				level := uint8(1)
				for _, change := range changes {
					if change.at <= t+interval/2 {
						level = change.level
					}
				}
				out = append(out, []uint8{0, level})
			}
			return out
		}
		_ = offset
		return []model.Chunk{
			{CaptureID: capture.ID, Index: 0, StartTime: 0, IntervalNs: interval, Levels: rows(0, 235000)},
			{CaptureID: capture.ID, Index: 1, ClockReset: true, StartTime: 0, IntervalNs: interval, Levels: rows(0, 235000)},
		}
	}
	chunksA, _ := st.AddChunks(build(captureA, 0))
	chunksB, _ := st.AddChunks(build(captureB, 5000))

	makeConfig := func(capture model.Capture) (model.Config, model.Result) {
		cfg, _ := st.CreateConfig(model.Config{CaptureID: capture.ID, Protocol: "uart", Name: "时钟重置 100k 8N1", Parameters: model.UARTParameters{Channel: 1, Baud: 100000, DataBits: 8, Parity: "none", StopBits: 1, IdleLevel: 1, ClockTolerance: 0.04}})
		result, _ := st.Recalculate(cfg.ID)
		return cfg, result
	}
	_, resultA := makeConfig(captureA)
	_, resultB := makeConfig(captureB)
	bytesA, bytesB := clockResetBytes(resultA), clockResetBytes(resultB)
	anchors := []model.AlignmentAnchor{}
	for i := 0; i < 4; i++ {
		baseSegment, otherSegment := "segment-1", "segment-1"
		if i >= 2 {
			baseSegment, otherSegment = "segment-2", "segment-2"
		}
		anchors = append(anchors, model.AlignmentAnchor{ID: fmt.Sprintf("reset-anchor-%d", i+1), Sequence: i + 1, BaseMarkerID: bytesA[i].ID, OtherMarkerID: bytesB[i].ID, BaseSegmentID: baseSegment, OtherSegmentID: otherSegment, BaseChunkID: bytesA[i].ChunkID, OtherChunkID: bytesB[i].ChunkID, BaseTimeNs: bytesA[i].StartNs, OtherTimeNs: bytesB[i].StartNs, UncertaintyNs: interval/2 + 1, ManualOrderConfirmed: true, Evidence: "人工核对重置轮次：段 1 A/B 在前，时钟二次从 0 开始后的段 2 A/C 在后"})
	}
	align := model.Alignment{Name: "时钟回绕/双重置/重复 A 报文核对", BaseCaptureID: captureA.ID, OtherCaptureID: captureB.ID, BaseResultID: resultA.ID, OtherResultID: resultB.ID, Anchors: anchors, Status: alignment.StatusReady}
	for _, a := range align.Anchors {
		bm := findCrossMarker(resultA.Markers, a.BaseMarkerID)
		for _, om := range resultB.Markers {
			if om.ID != a.OtherMarkerID && om.SegmentID == a.OtherSegmentID && sameCrossSignature(bm, om) {
				align.ExcludedCandidates = append(align.ExcludedCandidates, model.AlignmentCandidate{ID: a.ID + "-excluded", AnchorID: a.ID, BaseMarkerID: bm.ID, OtherMarkerID: om.ID, BaseSegmentID: bm.SegmentID, OtherSegmentID: om.SegmentID, BaseTimeNs: bm.StartNs, OtherTimeNs: om.StartNs, Status: alignment.CandidateExcluded, Reason: "重复 A 报文候选：必须在同一重置轮次内比较，不能跳到另一轮同名时间"})
			}
		}
	}
	_, _ = st.SaveAlignment(align)
	if first {
		return demoPayload(st, captureA, chunksA)
	}
	return demoPayload(st, captureB, chunksB)
}

func clockResetBytes(result model.Result) []model.Marker {
	out := []model.Marker{}
	for _, marker := range result.Markers {
		if marker.Kind == "byte" {
			out = append(out, marker)
		}
	}
	return out
}
