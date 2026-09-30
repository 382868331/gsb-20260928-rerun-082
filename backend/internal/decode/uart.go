package decode

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"logicstation/internal/model"
)

const resultSchemaVersion = 1

type UARTDecoder struct{}

type uartFrame struct {
	start          int64
	bitTime        int64
	dataBits       int
	stopBits       int
	parity         string
	channel        int
	idle           uint8
	tolerance      float64
	bits           []uint8
	raw            []string
	nextCenter     int64
	lastSampleTime int64
	sampleNs       int64
	clockError     int64
}

func (d *UARTDecoder) Validate(cfg model.Config) error {
	p := cfg.Parameters
	if p.Channel < 0 {
		return fmt.Errorf("channel must be non-negative")
	}
	if p.Baud <= 0 {
		return fmt.Errorf("baud must be positive")
	}
	if p.DataBits < 5 || p.DataBits > 9 {
		return fmt.Errorf("dataBits must be between 5 and 9")
	}
	if p.StopBits < 1 || p.StopBits > 2 {
		return fmt.Errorf("stopBits must be 1 or 2")
	}
	switch p.Parity {
	case "", "none", "even", "odd", "mark", "space":
	default:
		return fmt.Errorf("unsupported parity %q", p.Parity)
	}
	if p.ClockTolerance < 0 || p.ClockTolerance > 0.2 {
		return fmt.Errorf("clockTolerance must be between 0 and 0.2")
	}
	if p.IdleLevel > 1 {
		return fmt.Errorf("idleLevel must be 0 or 1")
	}
	return nil
}

func (d *UARTDecoder) Decode(captureName string, chunks []model.Chunk, cfg model.Config) model.Result {
	p := cfg.Parameters
	tolerance := p.ClockTolerance
	if tolerance == 0 {
		tolerance = 0.03
	}
	ordered := append([]model.Chunk(nil), chunks...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })

	result := model.Result{
		ID:            cfg.ID + ":result",
		CaptureID:     cfg.CaptureID,
		ConfigID:      cfg.ID,
		ConfigVersion: cfg.Version,
		Protocol:      "uart",
		SchemaVersion: resultSchemaVersion,
		CreatedAt:     time.Now().UTC(),
		Markers:       []model.Marker{},
	}

	bitTime := int64(math.Round(1e9 / float64(p.Baud)))
	var frame *uartFrame
	var haveLast bool
	var lastTime int64
	var lastLevel uint8
	markerSeq := 0
	addMarker := func(m model.Marker) {
		markerSeq++
		if m.ID == "" {
			m.ID = fmt.Sprintf("%s-m%d", cfg.ID, markerSeq)
		}
		result.Markers = append(result.Markers, m)
	}

	levelAt := func(chunk model.Chunk, sample int) (uint8, error) {
		if sample >= len(chunk.Levels) {
			return 0, fmt.Errorf("missing sample")
		}
		row := chunk.Levels[sample]
		if p.Channel >= len(row) {
			return 0, fmt.Errorf("channel %d is outside capture", p.Channel)
		}
		return row[p.Channel], nil
	}

	for _, chunk := range ordered {
		if len(chunk.Levels) == 0 {
			continue
		}
		if haveLast {
			expectedNext := lastTime + chunk.IntervalNs
			if chunk.StartTime != expectedNext {
				start := lastTime
				end := chunk.StartTime
				label := "采样间隙：缺失采样，不补全字节"
				if frame != nil {
					start = frame.start
					label = "采样间隙：跨块未完成帧不能接为正常字节"
					frame = nil
				}
				addMarker(model.Marker{Kind: "error", Code: "sampling-gap", Severity: "error", Channel: p.Channel, StartNs: start, EndNs: end, Label: label})
				haveLast = false
			}
		}

		for sampleIndex := range chunk.Levels {
			level, err := levelAt(chunk, sampleIndex)
			if err != nil {
				addMarker(model.Marker{Kind: "error", Code: "missing-channel", Severity: "error", Channel: p.Channel, StartNs: chunk.StartTime + int64(sampleIndex)*chunk.IntervalNs, EndNs: chunk.StartTime + int64(sampleIndex)*chunk.IntervalNs, Label: err.Error()})
				continue
			}
			t := chunk.StartTime + int64(sampleIndex)*chunk.IntervalNs

			if haveLast && lastLevel != level {
				if frame == nil && lastLevel == p.IdleLevel && level == 1-p.IdleLevel {
					frame = &uartFrame{start: t, bitTime: bitTime, dataBits: p.DataBits, stopBits: p.StopBits, parity: p.Parity, channel: p.Channel, idle: p.IdleLevel, tolerance: tolerance, nextCenter: t + bitTime/2, lastSampleTime: t, sampleNs: chunk.IntervalNs}
				} else if frame != nil {
					edge := t
					offset := (edge - frame.start) % frame.bitTime
					if offset < 0 {
						offset += frame.bitTime
					}
					distance := float64(min(offset, frame.bitTime-offset))
					if distance > frame.tolerance*float64(frame.bitTime)+float64(frame.sampleNs)/2 && frame.clockError == 0 {
						frame.clockError = edge
					}
				}
			}

			if frame != nil {
				for t >= frame.nextCenter {
					bitIndex := len(frame.bits)
					frame.bits = append(frame.bits, level)
					frame.raw = append(frame.raw, fmt.Sprint(level))
					frame.nextCenter += frame.bitTime
					totalBits := 1 + frame.dataBits + frame.stopBits
					if frame.parity != "" && frame.parity != "none" {
						totalBits++
					}
					if bitIndex == totalBits-1 {
						end := frame.start + int64(totalBits)*frame.bitTime
						evaluateFrame(addMarker, *frame, end)
						frame = nil
						break
					}
				}
			}

			lastTime, lastLevel, haveLast = t, level, true
		}
	}

	if frame != nil {
		addMarker(model.Marker{Kind: "error", Code: "incomplete-frame", Severity: "error", Channel: p.Channel, StartNs: frame.start, EndNs: lastTime, Label: "报文在采集结束前未完成", RawBits: strings.Join(frame.raw, "")})
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

func evaluateFrame(add func(model.Marker), frame uartFrame, end int64) {
	raw := strings.Join(frame.raw, "")
	dataStart := 1
	dataEnd := dataStart + frame.dataBits
	if len(frame.bits) < frame.dataBits+frame.stopBits {
		add(model.Marker{Kind: "error", Code: "incomplete-frame", Severity: "error", Channel: frame.channel, StartNs: frame.start, EndNs: end, Label: "帧位数不足", RawBits: raw})
		return
	}
	if frame.clockError != 0 {
		add(model.Marker{Kind: "error", Code: "clock-anomaly", Severity: "error", Channel: frame.channel, StartNs: frame.start, EndNs: end, Label: "时钟异常：边沿偏离 UART 位边界", RawBits: raw})
		return
	}

	var value byte
	for i := dataStart; i < dataEnd; i++ {
		logicalOne := frame.idle
		if frame.bits[i] == logicalOne {
			value |= 1 << (i - dataStart)
		}
	}

	parityIndex := dataEnd
	if frame.parity != "" && frame.parity != "none" {
		got := frame.bits[parityIndex]
		var ones int
		for _, bit := range frame.bits[dataStart:dataEnd] {
			if bit == frame.idle {
				ones++
			}
		}
		want := uint8(0)
		switch frame.parity {
		case "even":
			want = uint8(ones % 2)
		case "odd":
			want = uint8(1 - ones%2)
		case "mark":
			want = frame.idle
		case "space":
			want = 1 - frame.idle
		}
		if got != want {
			add(model.Marker{Kind: "error", Code: "parity-error", Severity: "error", Channel: frame.channel, StartNs: frame.start, EndNs: end, Label: "校验位错误，不输出正常字节", RawBits: raw})
			return
		}
	}

	stopStart := parityIndex
	if frame.parity == "" || frame.parity == "none" {
		stopStart = dataEnd
	} else {
		stopStart++
	}
	for i := stopStart; i < stopStart+frame.stopBits; i++ {
		if frame.bits[i] != frame.idle {
			add(model.Marker{Kind: "error", Code: "framing-error", Severity: "error", Channel: frame.channel, StartNs: frame.start, EndNs: end, Label: "停止位不是空闲电平，不输出正常字节", RawBits: raw})
			return
		}
	}

	v := value
	add(model.Marker{Kind: "byte", Severity: "info", Channel: frame.channel, StartNs: frame.start, EndNs: end, Label: fmt.Sprintf("0x%02X (%q)", value, safeRune(value)), Value: &v, RawBits: raw})
}

func safeRune(v byte) rune {
	if v >= 32 && v < 127 {
		return rune(v)
	}
	return '.'
}
