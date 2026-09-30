package decode

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"logicstation/internal/model"
)

type SPIDecoder struct{}

func (d *SPIDecoder) Validate(cfg model.Config) error {
	p := cfg.SPI
	if p == nil {
		return fmt.Errorf("spi parameters are required")
	}
	if p.ClockChannel < 0 || p.DataChannel < 0 {
		return fmt.Errorf("clock and data channels must be non-negative")
	}
	if p.ClockChannel == p.DataChannel {
		return fmt.Errorf("clock and data channels must differ")
	}
	if p.CSChannel >= 0 && (p.CSChannel == p.ClockChannel || p.CSChannel == p.DataChannel) {
		return fmt.Errorf("cs channel must differ from clock and data channels")
	}
	if p.CSActive > 1 {
		return fmt.Errorf("csActive must be 0 or 1")
	}
	switch p.BitOrder {
	case "msb", "lsb":
	default:
		return fmt.Errorf("bitOrder must be \"msb\" or \"lsb\"")
	}
	switch p.ClockEdge {
	case "rising", "falling":
	default:
		return fmt.Errorf("clockEdge must be \"rising\" or \"falling\"")
	}
	if p.BitsPerWord < 4 || p.BitsPerWord > 32 {
		return fmt.Errorf("bitsPerWord must be between 4 and 32")
	}
	return nil
}

func (d *SPIDecoder) Decode(captureName string, chunks []model.Chunk, cfg model.Config) model.Result {
	p := cfg.SPI
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
	var wordStart int64
	wordOpen := false
	discardWord := func(end int64, code, label string) {
		if !wordOpen {
			return
		}
		raw := make([]string, len(bits))
		for i, bit := range bits {
			raw[i] = fmt.Sprint(bit)
		}
		addMarker(model.Marker{
			Kind: "error", Code: code, Severity: "error", Channel: p.DataChannel,
			StartNs: wordStart, EndNs: end, Label: label, RawBits: strings.Join(raw, ""),
		})
		bits = nil
		wordOpen = false
	}

	var haveLast bool
	var lastTime int64
	var lastClock uint8

	for _, chunk := range ordered {
		if len(chunk.Levels) == 0 {
			continue
		}
		if haveLast && chunk.StartTime != lastTime+chunk.IntervalNs {
			start := lastTime
			hadWord := wordOpen
			if wordOpen {
				start = wordStart
			}
			discardWord(chunk.StartTime, "sampling-gap", "采样间隙：缺失采样，不完整报文不补字节")
			if !hadWord {
				addMarker(model.Marker{Kind: "error", Code: "sampling-gap", Severity: "error", Channel: p.DataChannel, StartNs: start, EndNs: chunk.StartTime, Label: "采样间隙：缺失采样"})
			}
			haveLast = false
		}

		for sampleIndex, row := range chunk.Levels {
			t := chunk.StartTime + int64(sampleIndex)*chunk.IntervalNs
			if p.ClockChannel >= len(row) || p.DataChannel >= len(row) || (p.CSChannel >= 0 && p.CSChannel >= len(row)) {
				addMarker(model.Marker{Kind: "error", Code: "missing-channel", Severity: "error", Channel: p.DataChannel, StartNs: t, EndNs: t, Label: "配置通道超出捕获通道数"})
				lastTime, haveLast = t, true
				continue
			}
			clock := row[p.ClockChannel]
			data := row[p.DataChannel]
			csOK := true
			if p.CSChannel >= 0 {
				csOK = row[p.CSChannel] == p.CSActive
			}

			if !csOK && wordOpen {
				discardWord(t, "incomplete-frame", "片选中途结束：报文不完整，不补字节")
			}

			if haveLast && clock != lastClock && csOK {
				rising := lastClock == 0 && clock == 1
				sampleEdge := (p.ClockEdge == "rising" && rising) || (p.ClockEdge == "falling" && !rising)
				if sampleEdge {
					if !wordOpen {
						wordOpen = true
						wordStart = t
						bits = nil
					}
					bits = append(bits, data)
					if len(bits) == p.BitsPerWord {
						var value uint32
						if p.BitOrder == "msb" {
							for _, bit := range bits {
								value = value<<1 | uint32(bit)
							}
						} else {
							for i, bit := range bits {
								value |= uint32(bit) << i
							}
						}
						raw := make([]string, len(bits))
						for i, bit := range bits {
							raw[i] = fmt.Sprint(bit)
						}
						v := byte(value)
						addMarker(model.Marker{
							Kind: "byte", Severity: "info", Channel: p.DataChannel,
							StartNs: wordStart, EndNs: t,
							Label: fmt.Sprintf("0x%02X (%q)", v, safeRune(v)),
							Value: &v, RawBits: strings.Join(raw, ""),
						})
						bits = nil
						wordOpen = false
					}
				}
			}

			lastClock = clock
			lastTime, haveLast = t, true
		}
	}

	if wordOpen {
		discardWord(lastTime, "incomplete-frame", "报文在采集结束前未完成，不补字节")
	}

	for _, marker := range result.Markers {
		if marker.Kind == "byte" {
			result.ValidBytes++
		} else if marker.Severity == "error" {
			result.ErrorCount++
		}
	}
	return result
}
