package decode

import (
	"fmt"

	"logicstation/internal/model"
)

// uartDecoder is a sample-synchronous UART decoder.
//
// Timing is tracked against the configured baud rate in sample units. A
// falling edge of the (optionally inverted) line starts a start bit; each
// following data/parity/stop cell is sampled at its midpoint. Any edge during
// the frame that does not line up (within tolerance) with a bit-cell boundary
// is a clock anomaly: the frame is aborted and an error item is emitted
// instead of forcing a byte out. Capture gaps while a frame is open are
// handled the same way by NotifyGap.
type uartDecoder struct {
	cfg       model.DecodeConfig
	sampleCfg model.SampleConfig
	channel   string
	chanIdx   int
	periodNs  int64

	samplesPerBit float64
	tolerance     float64

	active      bool
	startSample int64
	startEdgeNs int64
	haveLast    bool
	lastLevel   int
	recorded    int
	frameBits   []model.Bit
	// After an abort or a gap, ignore edges until the line has sat idle (high)
	// for at least one bit cell, so glitch tails cannot start a phantom frame.
	cooldown int64
	idleRun  int64
}

func newUART(cfg model.DecodeConfig, sampleCfg model.SampleConfig) (*uartDecoder, error) {
	if cfg.BaudRate <= 0 {
		return nil, fmt.Errorf("uart: baudRate must be positive")
	}
	if cfg.DataBits == 0 {
		cfg.DataBits = 8
	}
	if cfg.DataBits < 5 || cfg.DataBits > 9 {
		return nil, fmt.Errorf("uart: dataBits must be 5..9, got %d", cfg.DataBits)
	}
	if cfg.Parity == "" {
		cfg.Parity = "none"
	}
	if cfg.Parity != "none" && cfg.Parity != "even" && cfg.Parity != "odd" {
		return nil, fmt.Errorf("uart: invalid parity %q", cfg.Parity)
	}
	if cfg.StopBits == 0 {
		cfg.StopBits = 1
	}

	channel := ""
	if len(cfg.Channels) > 0 {
		channel = cfg.Channels[0]
	}
	chanIdx := -1
	for i, name := range sampleCfg.Channels {
		if name == channel {
			chanIdx = i
			break
		}
	}
	if chanIdx < 0 {
		return nil, fmt.Errorf("uart: decode channel %q not present in sample channels %v", channel, sampleCfg.Channels)
	}
	tol := cfg.ClockTol
	if tol <= 0 {
		tol = 0.03
	}

	sampleRate := 1e9 / float64(sampleCfg.PeriodNs)
	d := &uartDecoder{
		cfg:           cfg,
		sampleCfg:     sampleCfg,
		channel:       channel,
		chanIdx:       chanIdx,
		periodNs:      sampleCfg.PeriodNs,
		samplesPerBit: sampleRate / float64(cfg.BaudRate),
		tolerance:     tol,
	}
	if d.samplesPerBit < 1 {
		return nil, fmt.Errorf("uart: sample rate %.0f Hz below baud rate %d (need >= 1 sample/bit)", sampleRate, cfg.BaudRate)
	}
	return d, nil
}

func (d *uartDecoder) level(raw byte) int {
	l := int((raw >> uint(d.chanIdx)) & 1)
	if d.cfg.Invert {
		l = 1 - l
	}
	return l
}

// framePos returns the frame position in bit cells at global sample s.
// Position 0 is the start-edge boundary one sample before startSample.
func (d *uartDecoder) framePos(s int64) float64 {
	return float64(s-(d.startSample-1)) / d.samplesPerBit
}

func (d *uartDecoder) bitName(idx int) string {
	switch {
	case idx == 0:
		return "start"
	case idx <= d.cfg.DataBits:
		return fmt.Sprintf("d%d", idx-1)
	case d.cfg.Parity != "none" && idx == d.cfg.DataBits+1:
		return "parity"
	default:
		return "stop"
	}
}

func (d *uartDecoder) totalBits() float64 {
	n := 1 + float64(d.cfg.DataBits)
	if d.cfg.Parity != "none" {
		n++
	}
	n += d.cfg.StopBits
	return n
}

// FeedBlock consumes one contiguous sample block.
func (d *uartDecoder) FeedBlock(block model.SampleBlock) []model.Item {
	var items []model.Item
	havePrev := d.haveLast
	prevLevel := d.lastLevel
	for i := 0; i < block.Count; i++ {
		s := block.StartIndex + int64(i)
		l := d.level(block.Levels[i])
		if d.cooldown > 0 {
			if l == 1 {
				d.idleRun++
				if d.idleRun >= d.cooldown {
					d.cooldown = 0
				}
			} else {
				d.idleRun = 0
			}
			d.lastLevel = l
			d.haveLast = true
			havePrev = true
			prevLevel = l
			continue
		}

		if d.active {
			// A transition inside the frame must land on a cell boundary.
			if l != d.lastLevel {
				pos := d.framePos(s)
				nearest, dist := nearestInt(pos)
				if nearest >= 0 && float64(nearest) < d.totalBits() && dist > d.tolerance {
					items = append(items, d.abort(model.ErrClockAnomaly,
						fmt.Sprintf("edge at %.3f bits, boundary %d off by %.3f bit (> %.2f)", pos, nearest, dist, d.tolerance), s))
				}
			}
			if d.active {
				items = append(items, d.advance(s, l)...)
			}
		}

		if !d.active && l == 0 && havePrev && prevLevel == 1 {
			d.active = true
			d.startSample = s
			d.startEdgeNs = (s - 1) * d.periodNs
			d.recorded = 0
			d.frameBits = d.frameBits[:0]
			items = append(items, d.advance(s, l)...)
		}

		d.lastLevel = l
		d.haveLast = true
		havePrev = true
		prevLevel = l
	}
	return items
}

// advance records every bit-cell midpoint crossed by sample s (that sample is
// the nearest one to the midpoint) and closes the frame after the stop cell.
// Cell i spans frame positions [i, i+1), midpoint i+0.5.
func (d *uartDecoder) advance(s int64, l int) []model.Item {
	pos := d.framePos(s)
	nCells := int(d.totalBits())
	midsPassed := int(pos + 0.5)
	if midsPassed > nCells {
		midsPassed = nCells
	}
	for d.recorded < midsPassed {
		cell := d.recorded
		atNs := d.startEdgeNs + int64((float64(cell)+0.5)*d.samplesPerBit*float64(d.periodNs)+0.5)
		d.frameBits = append(d.frameBits, model.Bit{Name: d.bitName(cell), Level: l, AtNs: atNs})
		d.recorded++
	}
	if pos >= d.totalBits() {
		return []model.Item{d.complete()}
	}
	return nil
}

// complete finalizes a frame whose stop cell has fully elapsed. Any failed
// check becomes an error item; no data byte is produced on failure.
func (d *uartDecoder) complete() model.Item {
	startNs := d.startEdgeNs
	endNs := d.startEdgeNs + int64(d.totalBits()*d.samplesPerBit*float64(d.periodNs)+0.5)
	bits := d.frameBits
	d.active = false
	d.frameBits = nil
	d.recorded = 0

	if bitLevel(bits, "start") != 0 {
		return model.Item{Kind: model.KindError, StartNs: startNs, EndNs: endNs, Channel: d.channel,
			ErrCode: model.ErrFraming, Message: "start bit sampled high", Bits: bits}
	}

	value := 0
	for i := 0; i < d.cfg.DataBits; i++ {
		if bitLevel(bits, fmt.Sprintf("d%d", i)) == 1 {
			value |= 1 << uint(i)
		}
	}

	if d.cfg.Parity != "none" {
		if !parityOK(value, d.cfg.DataBits, bitLevel(bits, "parity"), d.cfg.Parity) {
			return model.Item{Kind: model.KindError, StartNs: startNs, EndNs: endNs, Channel: d.channel,
				ErrCode: model.ErrParity, Message: fmt.Sprintf("%s parity mismatch, byte=0x%02X", d.cfg.Parity, value), Bits: bits}
		}
	}

	if bitLevel(bits, "stop") != 1 {
		return model.Item{Kind: model.KindError, StartNs: startNs, EndNs: endNs, Channel: d.channel,
			ErrCode: model.ErrFraming, Message: fmt.Sprintf("framing error: stop bit low, byte=0x%02X", value), Bits: bits}
	}

	v := value
	return model.Item{Kind: model.KindData, StartNs: startNs, EndNs: endNs, Channel: d.channel, Value: &v, Bits: bits}
}

// abort drops an open frame and emits an error covering it. Bits seen so far
// are deliberately discarded: an aborted frame is never a byte.
func (d *uartDecoder) abort(code, msg string, atSample int64) model.Item {
	startNs := d.startEdgeNs
	endNs := atSample * d.periodNs
	d.active = false
	d.frameBits = nil
	d.recorded = 0
	d.cooldown = int64(d.samplesPerBit + 0.5)
	d.idleRun = 0
	return model.Item{Kind: model.KindError, StartNs: startNs, EndNs: endNs, Channel: d.channel,
		ErrCode: code, Message: msg}
}

// NotifyGap handles a missing-sample interval. An open frame can never be
// completed reliably across unknown levels, so it ends as a sample_gap error.
func (d *uartDecoder) NotifyGap(startNs, endNs int64) []model.Item {
	if !d.active {
		d.haveLast = false
		return nil
	}
	item := model.Item{Kind: model.KindError, StartNs: d.startEdgeNs, EndNs: endNs, Channel: d.channel,
		ErrCode: model.ErrSampleGap, Message: fmt.Sprintf("sample gap %dns..%dns interrupted frame", startNs, endNs)}
	d.active = false
	d.frameBits = nil
	d.recorded = 0
	d.haveLast = false
	d.cooldown = 0
	d.idleRun = 0
	return []model.Item{item}
}

// Flush emits nothing: a frame that never reaches its stop cell is truncated,
// not a decodable byte.
func (d *uartDecoder) Flush() []model.Item { return nil }

func bitLevel(bits []model.Bit, name string) int {
	for _, b := range bits {
		if b.Name == name {
			return b.Level
		}
	}
	return 1
}

func parityOK(value, dataBits, p int, kind string) bool {
	ones := 0
	for i := 0; i < dataBits; i++ {
		ones += (value >> uint(i)) & 1
	}
	if kind == "odd" {
		return (ones+p)%2 == 1
	}
	return (ones+p)%2 == 0
}
