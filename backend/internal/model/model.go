package model

import "time"

type Chunk struct {
	ID         string    `json:"id"`
	CaptureID  string    `json:"captureId"`
	Index      int       `json:"index"`
	StartTime  int64     `json:"startTimeNs"`
	IntervalNs int64     `json:"intervalNs"`
	Levels     [][]uint8 `json:"levels"`
}

func (c Chunk) EndTime() int64 {
	if len(c.Levels) == 0 {
		return c.StartTime
	}
	return c.StartTime + int64(len(c.Levels)-1)*c.IntervalNs
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
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Code     string `json:"code,omitempty"`
	StartNs  int64  `json:"startNs"`
	EndNs    int64  `json:"endNs"`
	Channel  int    `json:"channel"`
	Label    string `json:"label"`
	Severity string `json:"severity"`
	Value    *byte  `json:"value,omitempty"`
	RawBits  string `json:"rawBits,omitempty"`
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

// AlignmentAnchor 是用户选定（或自动候选采纳）的跨采样共同事件：
// 同一通信事件在基准采样与对照采样各自时间轴上的位置。
type AlignmentAnchor struct {
	ID      string `json:"id"`
	Label   string `json:"label,omitempty"`
	BaseNs  int64  `json:"baseNs"`
	OtherNs int64  `json:"otherNs"`
	Source  string `json:"source,omitempty"`
}

// Alignment 是一组对齐依据的不可变版本。修改对齐依据会生成新版本，
// 旧版本及其锚点保留，可追溯每次比较使用的对齐点。
type Alignment struct {
	ID             string            `json:"id"`
	GroupID        string            `json:"groupId"`
	Name           string            `json:"name"`
	BaseCaptureID  string            `json:"baseCaptureId"`
	OtherCaptureID string            `json:"otherCaptureId"`
	Version        int               `json:"version"`
	Anchors        []AlignmentAnchor `json:"anchors"`
	CreatedAt      time.Time         `json:"createdAt"`
}
