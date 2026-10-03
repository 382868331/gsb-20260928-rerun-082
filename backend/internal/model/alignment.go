package model

import "time"

// AlignmentAnchor 是用户确认的一组跨采样共同事件。
type AlignmentAnchor struct {
	ID                   string `json:"id"`
	Sequence             int    `json:"sequence,omitempty"`
	BaseMarkerID         string `json:"baseMarkerId"`
	OtherMarkerID        string `json:"otherMarkerId"`
	BaseSegmentID        string `json:"baseSegmentId,omitempty"`
	OtherSegmentID       string `json:"otherSegmentId,omitempty"`
	BaseChunkID          string `json:"baseChunkId,omitempty"`
	OtherChunkID         string `json:"otherChunkId,omitempty"`
	BaseTimeNs           int64  `json:"baseTimeNs"`
	OtherTimeNs          int64  `json:"otherTimeNs"`
	UncertaintyNs        int64  `json:"uncertaintyNs"`
	ManualOrderConfirmed bool   `json:"manualOrderConfirmed,omitempty"`
	Evidence             string `json:"evidence,omitempty"`
}

// AlignmentCandidate 保存曾经参与匹配但未被采纳的候选，保证重开后可审计。
type AlignmentCandidate struct {
	ID             string `json:"id"`
	BaseMarkerID   string `json:"baseMarkerId"`
	OtherMarkerID  string `json:"otherMarkerId"`
	BaseSegmentID  string `json:"baseSegmentId,omitempty"`
	OtherSegmentID string `json:"otherSegmentId,omitempty"`
	BaseTimeNs     int64  `json:"baseTimeNs"`
	OtherTimeNs    int64  `json:"otherTimeNs"`
	Score          int    `json:"score"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
	AnchorID       string `json:"anchorId,omitempty"`
	PredictedNs    int64  `json:"predictedNs,omitempty"`
	ResidualNs     int64  `json:"residualNs,omitempty"`
	ConfidenceHalf int64  `json:"confidenceHalfWidthNs,omitempty"`
}

// AlignmentConflict records a contradiction between manually confirmed event
// order and the segment mapping. Existing anchors are never silently moved.
type AlignmentConflict struct {
	ID             string `json:"id"`
	FromAnchorID   string `json:"fromAnchorId"`
	ToAnchorID     string `json:"toAnchorId"`
	BaseSegmentID  string `json:"baseSegmentId"`
	OtherSegmentID string `json:"otherSegmentId"`
	Reason         string `json:"reason"`
}

// Alignment 是一次性比较所依据的不可变对齐版本；修改依据会创建新会话。
type Alignment struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	BaseCaptureID      string               `json:"baseCaptureId"`
	OtherCaptureID     string               `json:"otherCaptureId"`
	BaseResultID       string               `json:"baseResultId"`
	OtherResultID      string               `json:"otherResultId"`
	Anchors            []AlignmentAnchor    `json:"anchors"`
	ExcludedCandidates []AlignmentCandidate `json:"excludedCandidates"`
	Conflicts          []AlignmentConflict  `json:"conflicts,omitempty"`
	BaseSegments       []CaptureSegment     `json:"baseSegments,omitempty"`
	OtherSegments      []CaptureSegment     `json:"otherSegments,omitempty"`
	Status             string               `json:"status"`
	CreatedAt          time.Time            `json:"createdAt"`
}
