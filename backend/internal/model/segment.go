package model

import "sort"

// CaptureSegment 是一段时钟连续的采集。采集中途重置时钟（时间戳回绕、
// 重新从较小值开始）时，后续采样块属于新的段；各段独立与另一份采样建立
// 对应关系，不能把整份记录统一拉伸。
type CaptureSegment struct {
	Index    int      `json:"index"`
	StartNs  int64    `json:"startNs"`
	EndNs    int64    `json:"endNs"`
	ChunkIDs []string `json:"chunkIds"`
}

// SegmentChunks 按块索引顺序把采样划分为采集段：块起始时间早于前一块
// 末尾采样时间即为时钟重置，开启新段。普通的前跳/间隙仍属于同一段，
// 由解码器输出 sampling-gap 标记。
func SegmentChunks(chunks []Chunk) []CaptureSegment {
	ordered := append([]Chunk(nil), chunks...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })
	segments := []CaptureSegment{}
	for _, chunk := range ordered {
		if len(chunk.Levels) == 0 {
			continue
		}
		if len(segments) == 0 || chunk.StartTime < segments[len(segments)-1].EndNs {
			segments = append(segments, CaptureSegment{Index: len(segments)})
		}
		seg := &segments[len(segments)-1]
		if len(seg.ChunkIDs) == 0 {
			seg.StartNs = chunk.StartTime
		}
		if end := chunk.EndTime(); end > seg.EndNs || len(seg.ChunkIDs) == 0 {
			seg.EndNs = end
		}
		seg.ChunkIDs = append(seg.ChunkIDs, chunk.ID)
	}
	return segments
}

// SegmentIndexForTime 返回覆盖该时间的段。仅用于没有段信息的旧数据兜底；
// 新解码的标记直接携带 SegmentIndex，不做时间猜测。
func SegmentIndexForTime(segments []CaptureSegment, t int64) int {
	for _, seg := range segments {
		if t >= seg.StartNs && t <= seg.EndNs {
			return seg.Index
		}
	}
	if len(segments) == 0 {
		return 0
	}
	if t < segments[0].StartNs {
		return segments[0].Index
	}
	return segments[len(segments)-1].Index
}
