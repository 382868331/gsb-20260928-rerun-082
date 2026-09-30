package decode

import (
	"testing"

	"logicstation/internal/model"
)

type spiWave struct {
	cursor  int
	changes [3][]struct {
		at    int
		level uint8
	}
}

func newSPIWave() *spiWave {
	w := &spiWave{}
	w.set(0, 0)
	w.set(1, 0)
	w.set(2, 1)
	return w
}

func (w *spiWave) set(channel int, level uint8) {
	w.changes[channel] = append(w.changes[channel], struct {
		at    int
		level uint8
	}{w.cursor, level})
}

func (w *spiWave) idle(samples int) { w.cursor += samples }

func (w *spiWave) bit(bit uint8) {
	w.set(1, bit)
	w.idle(5)
	w.set(0, 1)
	w.idle(5)
	w.set(0, 0)
}

func (w *spiWave) sendByte(value byte) {
	for i := 7; i >= 0; i-- {
		w.bit((value >> i) & 1)
	}
}

func (w *spiWave) build(split int) []model.Chunk {
	const interval = int64(100)
	total := w.cursor + 4
	levels := make([][]uint8, total)
	for i := 0; i < total; i++ {
		row := make([]uint8, 3)
		for channel := 0; channel < 3; channel++ {
			level := uint8(0)
			if channel == 2 {
				level = 1
			}
			for _, change := range w.changes[channel] {
				if change.at <= i {
					level = change.level
				}
			}
			row[channel] = level
		}
		levels[i] = row
	}
	if split <= 0 || split >= total {
		split = total
	}
	return []model.Chunk{
		{ID: "c0", Index: 0, StartTime: 0, IntervalNs: interval, Levels: levels[:split]},
		{ID: "c1", Index: 1, StartTime: int64(split) * interval, IntervalNs: interval, Levels: levels[split:]},
	}
}

func spiTestConfig(order, edge string) model.Config {
	return model.Config{
		ID: "spi-cfg", CaptureID: "cap", Version: 1, Protocol: "spi",
		SPIParameters: &model.SPIParameters{
			ClockChannel: 0, DataChannel: 1, CSChannel: 2, CSActive: 0,
			ClockEdge: edge, BitOrder: order, BitsPerWord: 8,
		},
	}
}

func byteValues(result model.Result) []byte {
	var values []byte
	for _, marker := range result.Markers {
		if marker.Kind == "byte" && marker.Value != nil {
			values = append(values, *marker.Value)
		}
	}
	return values
}

func TestSPIKnownBytesAcrossChunks(t *testing.T) {
	w := newSPIWave()
	w.idle(20)
	w.set(2, 0)
	w.idle(5)
	w.sendByte(0x48)
	w.idle(5)
	w.set(2, 1)
	w.idle(20)
	w.set(2, 0)
	w.idle(5)
	w.sendByte(0x69)
	w.idle(5)
	w.set(2, 1)
	split := 20 + 5 + 80 + 5 + 20 + 5 + 30
	result := (&SPIDecoder{}).Decode("test", w.build(split), spiTestConfig("msb", "rising"))
	values := byteValues(result)
	if len(values) != 2 || values[0] != 0x48 || values[1] != 0x69 {
		t.Fatalf("expected 0x48 0x69 across chunks, got %#v (errors=%d)", values, result.ErrorCount)
	}
	if result.ErrorCount != 0 {
		t.Fatalf("expected no errors, got %#v", result.Markers)
	}
	second := result.Markers[1]
	if second.StartNs >= int64(split)*100 || second.EndNs <= int64(split)*100 {
		t.Fatalf("cross-chunk byte should straddle the boundary: %#v", second)
	}
}

func TestSPILSBConfigProducesIndependentResult(t *testing.T) {
	w := newSPIWave()
	w.idle(20)
	w.set(2, 0)
	w.idle(5)
	w.sendByte(0x48)
	w.idle(5)
	w.set(2, 1)
	chunks := w.build(0)
	msb := (&SPIDecoder{}).Decode("test", chunks, spiTestConfig("msb", "rising"))
	lsbCfg := spiTestConfig("lsb", "rising")
	lsbCfg.ID = "spi-cfg-lsb"
	lsbCfg.Version = 2
	lsb := (&SPIDecoder{}).Decode("test", chunks, lsbCfg)
	if got := byteValues(msb); len(got) != 1 || got[0] != 0x48 {
		t.Fatalf("msb decode wrong: %#v", got)
	}
	if got := byteValues(lsb); len(got) != 1 || got[0] != 0x12 {
		t.Fatalf("lsb decode should bit-reverse to 0x12, got %#v", got)
	}
	if lsb.ConfigID == msb.ConfigID || lsb.ConfigVersion == msb.ConfigVersion {
		t.Fatalf("results must keep their own config identity")
	}
}

func TestSPICSEndsMidByte(t *testing.T) {
	w := newSPIWave()
	w.idle(20)
	w.set(2, 0)
	w.idle(5)
	for i := 7; i >= 3; i-- {
		w.bit((0x55 >> i) & 1)
	}
	w.set(2, 1)
	w.idle(20)
	result := (&SPIDecoder{}).Decode("test", w.build(0), spiTestConfig("msb", "rising"))
	if result.ValidBytes != 0 {
		t.Fatalf("partial byte must not be fabricated: %#v", result.Markers)
	}
	if len(result.Markers) != 1 || result.Markers[0].Code != "incomplete-frame" || result.Markers[0].RawBits != "01010" {
		t.Fatalf("expected incomplete-frame with 5 raw bits, got %#v", result.Markers)
	}
}

func TestSPISamplingGapDropsPartialByte(t *testing.T) {
	w := newSPIWave()
	w.idle(20)
	w.set(2, 0)
	w.idle(5)
	for i := 7; i >= 4; i-- {
		w.bit((0x48 >> i) & 1)
	}
	chunks := w.build(45)
	chunks[1].StartTime += 5000
	result := (&SPIDecoder{}).Decode("test", chunks, spiTestConfig("msb", "rising"))
	if result.ValidBytes != 0 {
		t.Fatalf("gap must not produce a byte: %#v", result.Markers)
	}
	if len(result.Markers) == 0 || result.Markers[0].Code != "sampling-gap" {
		t.Fatalf("expected sampling-gap, got %#v", result.Markers)
	}
}

func TestSPIClockOutsideCSIsMarked(t *testing.T) {
	w := newSPIWave()
	w.idle(20)
	w.set(1, 1)
	w.idle(5)
	w.set(0, 1)
	w.idle(5)
	w.set(0, 0)
	w.idle(20)
	result := (&SPIDecoder{}).Decode("test", w.build(0), spiTestConfig("msb", "rising"))
	if result.ValidBytes != 0 || result.ErrorCount != 1 || result.Markers[0].Code != "clock-outside-cs" {
		t.Fatalf("expected clock-outside-cs error, got %#v", result.Markers)
	}
}
