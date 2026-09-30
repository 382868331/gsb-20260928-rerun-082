package decode

import (
	"testing"

	"logicstation/internal/model"
)

// spiWave 生成 mode-0 风格 SPI 采样：CS 低有效，数据在上升沿前稳定。
func spiWave(interval int64, half int, bytes []byte, csAbortAfter int) ([][]uint8, []int64) {
	rows := [][]uint8{}
	cur := []uint8{0, 0, 1}
	emit := func(n int) {
		for i := 0; i < n; i++ {
			rows = append(rows, append([]uint8(nil), cur...))
		}
	}
	set := func(clk, data, cs uint8) { cur = []uint8{clk, data, cs} }
	byteStarts := []int64{}
	emit(10)
	set(0, 0, 0)
	emit(4)
	for byteIndex, value := range bytes {
		if byteIndex > 0 {
			emit(4)
		}
		byteStarts = append(byteStarts, int64(len(rows))*interval+int64(half)*interval)
		limit := 8
		if byteIndex == len(bytes)-1 && csAbortAfter > 0 {
			limit = csAbortAfter
		}
		for bitIndex := 0; bitIndex < limit; bitIndex++ {
			bit := (value >> (7 - bitIndex)) & 1
			set(0, bit, 0)
			emit(half)
			set(1, bit, 0)
			emit(half)
			set(0, bit, 0)
		}
	}
	set(0, 0, 1)
	emit(10)
	return rows, byteStarts
}

func spiChunks(rows [][]uint8, interval int64, split int, gap bool) []model.Chunk {
	if split <= 0 || split >= len(rows) {
		return []model.Chunk{{ID: "c0", Index: 0, StartTime: 0, IntervalNs: interval, Levels: rows}}
	}
	secondStart := int64(split) * interval
	if gap {
		secondStart += interval * 30
	}
	return []model.Chunk{
		{ID: "c0", Index: 0, StartTime: 0, IntervalNs: interval, Levels: rows[:split]},
		{ID: "c1", Index: 1, StartTime: secondStart, IntervalNs: interval, Levels: rows[split:]},
	}
}

func spiConfig(edge, order string) model.Config {
	return model.Config{
		ID: "cfg-spi", CaptureID: "cap", Version: 1, Protocol: "spi",
		SPI: &model.SPIParameters{
			ClockChannel: 0, DataChannel: 1, CSChannel: 2, CSActive: 0,
			BitOrder: order, ClockEdge: edge, BitsPerWord: 8,
		},
	}
}

func byteValues(result model.Result) []byte {
	var out []byte
	for _, marker := range result.Markers {
		if marker.Kind == "byte" && marker.Value != nil {
			out = append(out, *marker.Value)
		}
	}
	return out
}

func TestSPIKnownBytesAndPositions(t *testing.T) {
	interval := int64(100)
	rows, starts := spiWave(interval, 8, []byte{0xA5, 0x3C}, 0)
	result := (&SPIDecoder{}).Decode("test", spiChunks(rows, interval, 0, false), spiConfig("rising", "msb"))

	got := byteValues(result)
	if len(got) != 2 || got[0] != 0xA5 || got[1] != 0x3C {
		t.Fatalf("expected [A5 3C], got % X (markers=%#v)", got, result.Markers)
	}
	if result.ErrorCount != 0 {
		t.Fatalf("expected no errors, got %#v", result.Markers)
	}
	for i, start := range starts {
		var markerStart int64
		for _, marker := range result.Markers {
			if marker.Kind == "byte" && marker.Value != nil && *marker.Value == []byte{0xA5, 0x3C}[i] {
				markerStart = marker.StartNs
			}
		}
		if markerStart != start {
			t.Fatalf("byte %d startNs=%d, want %d", i, markerStart, start)
		}
	}
}

func TestSPIByteContinuesAcrossChunkBoundary(t *testing.T) {
	interval := int64(100)
	rows, _ := spiWave(interval, 8, []byte{0xA5, 0x3C}, 0)
	split := 10 + 4 + 16*8 + 4 + 40 // 第二个字节中间
	result := (&SPIDecoder{}).Decode("test", spiChunks(rows, interval, split, false), spiConfig("rising", "msb"))
	got := byteValues(result)
	if len(got) != 2 || got[0] != 0xA5 || got[1] != 0x3C {
		t.Fatalf("cross-block byte lost: % X", got)
	}
	if result.ErrorCount != 0 {
		t.Fatalf("unexpected errors: %#v", result.Markers)
	}
}

func TestSPISamplingGapDoesNotFabricateByte(t *testing.T) {
	interval := int64(100)
	rows, _ := spiWave(interval, 8, []byte{0xA5, 0x3C}, 0)
	split := 10 + 4 + 16*8 + 4 + 40
	result := (&SPIDecoder{}).Decode("test", spiChunks(rows, interval, split, true), spiConfig("rising", "msb"))
	got := byteValues(result)
	if len(got) != 1 || got[0] != 0xA5 {
		t.Fatalf("gap must drop the split byte, got % X", got)
	}
	foundGap := false
	for _, marker := range result.Markers {
		if marker.Code == "sampling-gap" {
			foundGap = true
		}
	}
	if !foundGap {
		t.Fatalf("expected sampling-gap marker: %#v", result.Markers)
	}
}

func TestSPICSEarlyEndMarksIncomplete(t *testing.T) {
	interval := int64(100)
	rows, _ := spiWave(interval, 8, []byte{0xA5, 0x3C}, 5)
	result := (&SPIDecoder{}).Decode("test", spiChunks(rows, interval, 0, false), spiConfig("rising", "msb"))
	got := byteValues(result)
	if len(got) != 1 || got[0] != 0xA5 {
		t.Fatalf("CS abort must not fabricate the second byte, got % X", got)
	}
	found := false
	for _, marker := range result.Markers {
		if marker.Code == "incomplete-frame" && len(marker.RawBits) == 5 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected incomplete-frame with 5 raw bits: %#v", result.Markers)
	}
}

func TestSPIBitOrderAndWrongEdgeComparison(t *testing.T) {
	interval := int64(100)
	rows, _ := spiWave(interval, 8, []byte{0xA5}, 0)
	chunks := spiChunks(rows, interval, 0, false)

	lsb := (&SPIDecoder{}).Decode("test", chunks, spiConfig("rising", "lsb"))
	if got := byteValues(lsb); len(got) != 1 || got[0] != 0xA5 { // 0xA5 位反转仍是 0xA5
		t.Fatalf("lsb decode: % X", got)
	}

	wrong := (&SPIDecoder{}).Decode("test", chunks, spiConfig("falling", "msb"))
	right := (&SPIDecoder{}).Decode("test", chunks, spiConfig("rising", "msb"))
	gotWrong := byteValues(wrong)
	gotRight := byteValues(right)
	if len(gotRight) != 1 || gotRight[0] != 0xA5 {
		t.Fatalf("rising edge should decode 0xA5, got % X", gotRight)
	}
	if len(gotWrong) == 1 && gotWrong[0] == 0xA5 {
		t.Fatalf("wrong edge config must not reproduce the sent byte")
	}
}
