package model

import (
	"fmt"
	"time"
)

type Chunk struct {
	ID         string    `json:"id"`
	CaptureID  string    `json:"captureId"`
	Index      int       `json:"index"`
	SegmentID  string    `json:"segmentId,omitempty"`
	ClockReset bool      `json:"clockReset,omitempty"`
	StartTime  int64     `json:"startTimeNs"`
	IntervalNs int64     `json:"intervalNs"`
	Levels     [][]uint8 `json:"levels"`
}

// CaptureSegment 是按块物理顺序切出的采集段。时钟回绕/重置后，
// 后一段的 StartTimeNs 可以小于前一段，因此段身份不能由时间值代替。
type CaptureSegment struct {
	ID          string   `json:"id"`
	CaptureID   string   `json:"captureId"`
	Sequence    int      `json:"sequence"`
	ChunkIDs    []string `json:"chunkIds"`
	StartNs     int64    `json:"startNs"`
	EndNs       int64    `json:"endNs"`
	ClockReset  bool     `json:"clockReset"`
	ResetReason string   `json:"resetReason,omitempty"`
}

func (c Chunk) EndTime() int64 {
	if len(c.Levels) == 0 {
		return c.StartTime
	}
	return c.StartTime + int64(len(c.Levels)-1)*c.IntervalNs
}

// NormalizeChunkSegments assigns durable segment IDs using physical chunk order,
// not timestamp order. A lower StartTime after a later chunk is a clock reset.
func NormalizeChunkSegments(chunks []Chunk) {
	segmentSeq := 0
	var previous *Chunk
	var activeSegment string
	for i := range chunks {
		c := &chunks[i]
		reset := c.ClockReset
		if previous != nil {
			if c.SegmentID != "" && previous.SegmentID != "" && c.SegmentID != previous.SegmentID {
				reset = true
			}
			if c.StartTime < previous.StartTime {
				reset = true
			}
		}
		if previous == nil || reset {
			if previous != nil {
				segmentSeq++
			}
			if c.SegmentID == "" || (previous != nil && c.SegmentID == previous.SegmentID) {
				activeSegment = segmentID(segmentSeq)
			} else {
				activeSegment = c.SegmentID
			}
		}
		if c.SegmentID != "" {
			activeSegment = c.SegmentID
		}
		c.SegmentID = activeSegment
		c.ClockReset = reset
		previous = c
	}
}

func segmentID(seq int) string {
	return fmt.Sprintf("segment-%d", seq+1)
}

// CaptureSegments summarizes normalized chunks for alignment audit trails.
func CaptureSegments(captureID string, chunks []Chunk) []CaptureSegment {
	segments := []CaptureSegment{}
	indexByID := map[string]int{}
	for _, c := range chunks {
		if len(c.Levels) == 0 {
			continue
		}
		id := c.SegmentID
		if id == "" {
			id = segmentID(0)
		}
		idx, ok := indexByID[id]
		if !ok {
			idx = len(segments)
			indexByID[id] = idx
			segments = append(segments, CaptureSegment{ID: id, CaptureID: captureID, Sequence: idx, ChunkIDs: []string{}, StartNs: c.StartTime, EndNs: c.EndTime(), ClockReset: c.ClockReset})
		}
		seg := &segments[idx]
		if c.ID != "" {
			seg.ChunkIDs = append(seg.ChunkIDs, c.ID)
		}
		if c.StartTime < seg.StartNs {
			seg.StartNs = c.StartTime
		}
		if end := c.EndTime(); end > seg.EndNs {
			seg.EndNs = end
		}
	}
	return segments
}

type Capture struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ChannelNames []string  `json:"channelNames"`
	CreatedAt    time.Time `json:"createdAt"`
}

type UARTParameters struct {
	Channel        int     `json:"channel"`
	Baud           int     `json:"baud"`
	DataBits       int     `json:"dataBits"`
	Parity         string  `json:"parity"`
	StopBits       int     `json:"stopBits"`
	IdleLevel      uint8   `json:"idleLevel"`
	ClockTolerance float64 `json:"clockTolerance"`
}

// SPIParameters 描述时钟同步串行（SPI 风格）解码配置。
// CSChannel 为 -1 表示不使用片选，整个记录视为连续报文。
type SPIParameters struct {
	ClockChannel int    `json:"clockChannel"`
	DataChannel  int    `json:"dataChannel"`
	CSChannel    int    `json:"csChannel"`
	CSActive     uint8  `json:"csActive"`
	ClockEdge    string `json:"clockEdge"`
	BitOrder     string `json:"bitOrder"`
	BitsPerWord  int    `json:"bitsPerWord"`
}

type Config struct {
	ID            string         `json:"id"`
	CaptureID     string         `json:"captureId"`
	Version       int            `json:"version"`
	Protocol      string         `json:"protocol"`
	Name          string         `json:"name"`
	Parameters    UARTParameters `json:"parameters"`
	SPIParameters *SPIParameters `json:"spiParameters,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type Marker struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Code      string `json:"code,omitempty"`
	SegmentID string `json:"segmentId,omitempty"`
	ChunkID   string `json:"chunkId,omitempty"`
	StartNs   int64  `json:"startNs"`
	EndNs     int64  `json:"endNs"`
	Channel   int    `json:"channel"`
	Label     string `json:"label"`
	Severity  string `json:"severity"`
	Value     *byte  `json:"value,omitempty"`
	RawBits   string `json:"rawBits,omitempty"`
}

type Result struct {
	ID            string    `json:"id"`
	CaptureID     string    `json:"captureId"`
	ConfigID      string    `json:"configId"`
	ConfigVersion int       `json:"configVersion"`
	Protocol      string    `json:"protocol"`
	SchemaVersion int       `json:"schemaVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	ValidBytes    int       `json:"validBytes"`
	ErrorCount    int       `json:"errorCount"`
	Markers       []Marker  `json:"markers"`
}
