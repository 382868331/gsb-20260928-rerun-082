package decode

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"logicstation/internal/model"
)

// SPIDecoder 解码时钟同步串行信号：在片选有效区间内，
// 按配置的时钟边沿采样数据线，按位序组装字节。
type SPIDecoder struct{}

func (d *SPIDecoder) Validate(cfg model.Config) error {
	p := cfg.SPIParameters
	if p == nil {
		return fmt.Errorf("spi protocol requires spiParameters")
	}
	if p.ClockChannel < 0 || p.DataChannel < 0 {
		return fmt.Errorf("clock and data channels must be non-negative")
	}
	if p.ClockChannel == p.DataChannel {
		return fmt.Errorf("clock and data channels must differ")
	}
	if p.CSChannel < -1 {
		return fmt.Errorf("csChannel must be -1 (unused) or a valid channel")
	}
	if p.CSChannel == p.ClockChannel || p.CSChannel == p.DataChannel {
		return fmt.Errorf("cs channel must differ from clock and data channels")
	}
	if p.CSActive > 1 {
		return fmt.Errorf("csActive must be 0 or 1")
	}
	switch p.ClockEdge {
	case "rising", "falling":
	default:
		return fmt.Errorf("clockEdge must be rising or falling")
	}
	switch p.BitOrder {
	case "msb", "lsb":
	default:
		return fmt.Errorf("bitOrder must be msb or lsb")
	}
	if p.BitsPerWord < 4 || p.BitsPerWord > 64 {
		return fmt.Errorf("bitsPerWord must be between 4 and 64")
	}
	return nil
}

func (d *SPIDecoder) Decode(captureName string, chunks []model.Chunk, cfg model.Config) model.Result {
	p := cfg.SPIParameters
	ordered := append([]model.Chunk(nil), chunks...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })

	result := model.Result{
		ID:            cfg.ID + ":result",
		CaptureID:     cfg.CaptureID,
		ConfigID:      cfg.ID,
		ConfigVersion: cfg.Version,
		Protocol:      "spi",
		SchemaVersion: resultSchemaVersion,
		CreatedAt:     time.Now().UTC(),
		Markers:       []model.Marker{},
	}
	markerSeq := 0
	addMarker := func(m model.Marker) {
		markerSeq++
		if m.ID == "" {
			m.ID = fmt.Sprintf("%s-m%d", cfg.ID, markerSeq)
		}
		result.Markers = append(result.Markers, m)
	}

	var bits []uint8
	var byteStart int64
	var haveLast bool
	var lastTime int64
	var lastClock, lastCS uint8
	csActive := p.CSChannel < 0

	flushWithCode := func(end int64, code, label string) {
		if len(bits) == 0 {
			return
		}
		raw := make([]string, len(bits))
		for i, bit := range bits {
			raw[i] = fmt.Sprint(bit)
		}
		addMarker(model.Marker{
			Kind: "error", Code: code, Severity: "error",
			Channel: p.DataChannel, StartNs: byteStart, EndNs: end,
			Label: label, RawBits: strings.Join(raw, ""),
		})
		bits = nil
	}

	flushIncomplete := func(end int64, label string) {
		flushWithCode(end, "incomplete-frame", label)
	}

	for _, chunk := range ordered {
		if len(chunk.Levels) == 0 {
			continue
		}
		if haveLast {
			expectedNext := lastTime + chunk.IntervalNs
			if chunk.StartTime != expectedNext {
				if len(bits) > 0 {
					flushWithCode(chunk.StartTime, "sampling-gap", "采样间隙：跨块未完成字节不能接为正常字节")
				} else {
					addMarker(model.Marker{Kind: "error", Code: "sampling-gap", Severity: "error", Channel: p.DataChannel, StartNs: lastTime, EndNs: chunk.StartTime, Label: "采样间隙：缺失采样，不补全字节"})
				}
				haveLast = false
				csActive = p.CSChannel < 0
			}
		}

		for sampleIndex, row := range chunk.Levels {
			t := chunk.StartTime + int64(sampleIndex)*chunk.IntervalNs
			if p.ClockChannel >= len(row) || p.DataChannel >= len(row) || (p.CSChannel >= 0 && p.CSChannel >= len(row)) {
				addMarker(model.Marker{Kind: "error", Code: "missing-channel", Severity: "error", Channel: p.DataChannel, StartNs: t, EndNs: t, Label: "配置通道超出捕获通道数"})
				continue
			}
			clock := row[p.ClockChannel]
			data := row[p.DataChannel]

			if p.CSChannel >= 0 {
				cs := row[p.CSChannel]
				if haveLast && cs != lastCS {
					if cs == p.CSActive {
						csActive = true
					} else {
						csActive = false
						flushIncomplete(t, "片选在字节中途结束，不补全字节")
					}
				}
				lastCS = cs
			}

			if haveLast && clock != lastClock {
				samplingEdge := (p.ClockEdge == "rising" && clock == 1) || (p.ClockEdge == "falling" && clock == 0)
				if samplingEdge {
					if csActive {
						if len(bits) == 0 {
							byteStart = t
						}
						bits = append(bits, data)
						if len(bits) == p.BitsPerWord {
							var value byte
							for i, bit := range bits {
								if bit == 1 {
									if p.BitOrder == "msb" {
										value |= 1 << (p.BitsPerWord - 1 - i)
									} else {
										value |= 1 << i
									}
								}
							}
							raw := make([]string, len(bits))
							for i, bit := range bits {
								raw[i] = fmt.Sprint(bit)
							}
							v := value
							addMarker(model.Marker{
								Kind: "byte", Severity: "info",
								Channel: p.DataChannel, StartNs: byteStart, EndNs: t,
								Label: fmt.Sprintf("0x%02X (%q)", value, safeRune(value)),
								Value: &v, RawBits: strings.Join(raw, ""),
							})
							bits = nil
						}
					} else {
						addMarker(model.Marker{
							Kind: "error", Code: "clock-outside-cs", Severity: "error",
							Channel: p.ClockChannel, StartNs: t, EndNs: t,
							Label: "片选无效期间出现采样时钟边沿",
						})
					}
				}
			}

			lastTime, lastClock, haveLast = t, clock, true
		}
	}

	flushIncomplete(lastTime, "报文在采集结束前未完成")

	for _, marker := range result.Markers {
		if marker.Kind == "byte" {
			result.ValidBytes++
		} else if marker.Severity == "error" {
			result.ErrorCount++
		}
	}
	return result
}
