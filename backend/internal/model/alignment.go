package model

import "time"

// AlignmentAnchor 是用户确认的一组跨采样共同事件。
type AlignmentAnchor struct {
	ID            string `json:"id"`
	BaseMarkerID  string `json:"baseMarkerId"`
	OtherMarkerID string `json:"otherMarkerId"`
	BaseTimeNs    int64  `json:"baseTimeNs"`
	OtherTimeNs   int64  `json:"otherTimeNs"`
	UncertaintyNs int64  `json:"uncertaintyNs"`
	// 锚点两端各自所在的采集段。时钟重置后相同时间戳属于不同段，
	// 映射只在同一段对内建立，绝不跨段拉伸。
	BaseSegmentIndex  int `json:"baseSegmentIndex"`
	OtherSegmentIndex int `json:"otherSegmentIndex"`
}

// AlignmentCandidate 保存曾经参与匹配但未被采纳的候选，保证重开后可审计。
type AlignmentCandidate struct {
	ID             string `json:"id"`
	BaseMarkerID   string `json:"baseMarkerId"`
	OtherMarkerID  string `json:"otherMarkerId"`
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
	Status             string               `json:"status"`
	CreatedAt          time.Time            `json:"createdAt"`
}

// AlignmentConflict 指出一次人工确认与既有对齐证据的矛盾。
// 冲突只被报告，旧对齐决定保持不可变，绝不静默改动。
type AlignmentConflict struct {
	Kind                string `json:"kind"`
	Message             string `json:"message"`
	PreviousAlignmentID string `json:"previousAlignmentId,omitempty"`
	PreviousAnchorID    string `json:"previousAnchorId,omitempty"`
	NewAnchorID         string `json:"newAnchorId,omitempty"`
	OtherAnchorID       string `json:"otherAnchorId,omitempty"`
	BaseSegmentIndex    int    `json:"baseSegmentIndex"`
	OtherSegmentIndex   int    `json:"otherSegmentIndex"`
}
