package model

import "time"

// Timestamp is nanoseconds since capture start (t0 = 0).
type Timestamp int64

func (t Timestamp) Duration() time.Duration { return time.Duration(t) }

// SampleConfig is the immutable description of how raw levels were sampled.
// A new version is created whenever the sampling setup changes; sample blocks
// always reference the config version they were taken under.
type SampleConfig struct {
	Version   string   `json:"version"`
	Channels  []string `json:"channels"`
	PeriodNs  int64    `json:"periodNs"`
	CreatedAt string   `json:"createdAt"`
	Comment   string   `json:"comment,omitempty"`
}

// SampleBlock is one contiguous chunk of uniformly spaced manual level samples.
// Levels holds one byte per sample, each bit corresponding to one channel
// (channel i -> bit i). A missing chunk (time gap) is represented by the next
// block's StartIndex being greater than previous EndIndex+1; blocks MUST be
// appended in order.
type SampleBlock struct {
	ConfigVersion string `json:"configVersion"`
	StartIndex    int64  `json:"startIndex"`
	Count         int    `json:"count"`
	Levels        []byte `json:"levels"`
}

// StartNs returns the timestamp of the first sample.
func (b SampleBlock) StartNs(cfg *SampleConfig) int64 {
	return b.StartIndex * cfg.PeriodNs
}

// EndNs returns the timestamp one sample-period after the last sample
// (exclusive end of the covered interval).
func (b SampleBlock) EndNs(cfg *SampleConfig) int64 {
	return (b.StartIndex + int64(b.Count)) * cfg.PeriodNs
}

// DecodeConfig configures one protocol decoder run. Changing it creates a new
// version; results from older versions remain available for comparison.
type DecodeConfig struct {
	Version   string   `json:"version"`
	Protocol  string   `json:"protocol"` // currently only "uart"
	Channels  []string `json:"channels"` // protocol-specific channel roles, e.g. ["tx"]
	BaudRate  int      `json:"baudRate,omitempty"`
	DataBits  int      `json:"dataBits,omitempty"`       // 5..9, default 8
	Parity    string   `json:"parity,omitempty"`         // "none" | "even" | "odd", default none
	StopBits  float64  `json:"stopBits,omitempty"`       // 1 | 1.5 | 2, default 1
	Invert    bool     `json:"invert,omitempty"`         // invert raw levels (e.g. RS232)
	ClockTol  float64  `json:"clockTolerance,omitempty"` // allowed edge/bit-clock deviation, default 0.03
	CreatedAt string   `json:"createdAt"`
	Comment   string   `json:"comment,omitempty"`
}

const (
	KindData  = "data"
	KindError = "error"
)

// Error kinds. They are first-class results, never silently repaired into bytes.
const (
	ErrSampleGap       = "sample_gap"    // capture gap while a frame was open
	ErrClockAnomaly    = "clock_anomaly" // edge deviates beyond clock tolerance
	ErrFraming         = "framing"       // stop bit not at idle level
	ErrParity          = "parity"        // parity bit mismatch
	ErrIdleDuringFrame = "idle_resync"   // line went idle before frame completed
)

// Item is one decoded marker: either a successfully decoded data frame or an
// error annotation. Errors intentionally do not carry a decoded byte value.
type Item struct {
	Kind    string `json:"kind"`
	StartNs int64  `json:"startNs"`
	EndNs   int64  `json:"endNs"`
	Channel string `json:"channel"`
	Value   *int   `json:"value,omitempty"` // decoded byte (Kind == data only)
	Bits    []Bit  `json:"bits,omitempty"`  // per-bit timing, data frames only
	ErrCode string `json:"errCode,omitempty"`
	Message string `json:"message,omitempty"`
}

// Bit records where one UART bit cell was sampled (mid-cell instant).
type Bit struct {
	Name  string `json:"name"`  // start, d0..dN, parity, stop
	Level int    `json:"level"` // decoded (post-invert) level
	AtNs  int64  `json:"atNs"`
}

// DecodeResult is an immutable, versioned decoder output.
type DecodeResult struct {
	Version         string `json:"version"`
	SampleConfigVer string `json:"sampleConfigVersion"`
	DecodeConfigVer string `json:"decodeConfigVersion"`
	Protocol        string `json:"protocol"`
	Items           []Item `json:"items"`
	Bytes           []int  `json:"bytes"`
	ErrorCount      int    `json:"errorCount"`
	ComputedAt      string `json:"computedAt"`
}
