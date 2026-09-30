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

type SPIParameters struct {
	ClockChannel int    `json:"clockChannel"`
	DataChannel  int    `json:"dataChannel"`
	CSChannel    int    `json:"csChannel"`
	CSActive     uint8  `json:"csActive"`
	BitOrder     string `json:"bitOrder"`
	ClockEdge    string `json:"clockEdge"`
	BitsPerWord  int    `json:"bitsPerWord"`
}

type Config struct {
	ID         string         `json:"id"`
	CaptureID  string         `json:"captureId"`
	Version    int            `json:"version"`
	Protocol   string         `json:"protocol"`
	Name       string         `json:"name"`
	Parameters UARTParameters `json:"parameters"`
	SPI        *SPIParameters `json:"spiParameters,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
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
