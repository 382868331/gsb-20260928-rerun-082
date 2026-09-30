package api

import (
	"sort"

	"logicstation/internal/model"
	"logicstation/internal/store"
)

type change struct {
	at    int64
	level uint8
}

func CreateDemo(st *store.Store) map[string]any {
	return map[string]any{
		"uart": createUARTDemo(st),
		"spi":  createSPIDemo(st),
	}
}

func createUARTDemo(st *store.Store) map[string]any {
	capture := st.CreateCapture("内置 UART 核对波形", []string{"CLK-REF", "RX"})
	baud := 9600
	interval := int64(1000000000 / (baud * 4))
	bit := int64(1000000000 / baud)
	changes := []change{}
	var cursor int64 = bit * 2
	addByte := func(value byte) {
		changes = append(changes, change{cursor, 0})
		for i := 0; i < 8; i++ {
			changes = append(changes, change{cursor + bit*int64(i+1), (value >> i) & 1})
		}
		changes = append(changes, change{cursor + bit*9, 1})
		cursor += bit * 10
	}
	addByte('A')
	cursor += bit / 2
	addByte('B')
	changes = append(changes, change{cursor - bit, 0})
	totalSamples := int(cursor/interval) + 8
	split := int(float64(bit) * 22.75 / float64(interval))
	sort.Slice(changes, func(i, j int) bool { return changes[i].at < changes[j].at })

	build := func(offset, count, index int, start int64) model.Chunk {
		levels := make([][]uint8, count)
		for i := 0; i < count; i++ {
			t := int64(offset+i) * interval
			level := uint8(1)
			for _, change := range changes {
				if change.at <= t+interval/2 {
					level = change.level
				}
			}
			levels[i] = []uint8{0, level}
		}
		return model.Chunk{CaptureID: capture.ID, Index: index, StartTime: start, IntervalNs: interval, Levels: levels}
	}
	chunk0 := build(0, split, 0, 0)
	chunk1 := build(split, totalSamples-split, 1, int64(split)*interval)
	chunks, _ := st.AddChunks([]model.Chunk{chunk0, chunk1})
	cfg, _ := st.CreateConfig(model.Config{
		CaptureID: capture.ID,
		Protocol:  "uart",
		Name:      "9600 8N1 RX",
		Parameters: model.UARTParameters{
			Channel: 1, Baud: baud, DataBits: 8, Parity: "none", StopBits: 1, IdleLevel: 1, ClockTolerance: 0.03,
		},
	})
	result, _ := st.Recalculate(cfg.ID)
	return map[string]any{"capture": capture, "chunks": chunks, "configs": []model.Config{cfg}, "results": []model.Result{result}}
}

// createSPIDemo 生成时钟同步串行核对波形：已知字节 0xA5、跨块报文 0x3C，
// 以及一个片选中途结束的不完整报文。两个解码配置（上升沿/下降沿采样）共用
// 同一采样记录，结果各自保留用于对照。
func createSPIDemo(st *store.Store) map[string]any {
	capture := st.CreateCapture("内置 SPI 核对波形", []string{"CLK", "MOSI", "CS"})
	interval := int64(100)
	half := 8

	rows := [][]uint8{}
	cur := []uint8{0, 0, 1}
	emit := func(n int) {
		for i := 0; i < n; i++ {
			rows = append(rows, append([]uint8(nil), cur...))
		}
	}
	set := func(clk, data, cs uint8) { cur = []uint8{clk, data, cs} }
	emitBit := func(bit uint8) {
		set(0, bit, 0)
		emit(half)
		set(1, bit, 0)
		emit(half)
		set(0, bit, 0)
	}
	emitByte := func(value byte) {
		for bitIndex := 7; bitIndex >= 0; bitIndex-- {
			emitBit((value >> bitIndex) & 1)
		}
	}

	emit(20)
	set(0, 0, 0)
	emit(4)
	emitByte(0xA5)
	emit(8)
	byte2Start := len(rows)
	emitByte(0x3C)
	set(0, 0, 1)
	emit(16)
	set(0, 0, 0)
	emit(4)
	for bitIndex := 0; bitIndex < 5; bitIndex++ {
		emitBit(1)
	}
	set(0, 0, 1)
	emit(20)
	set(0, 0, 1)
	emit(20)

	split := byte2Start + 40
	chunk0 := model.Chunk{CaptureID: capture.ID, Index: 0, StartTime: 0, IntervalNs: interval, Levels: rows[:split]}
	chunk1 := model.Chunk{CaptureID: capture.ID, Index: 1, StartTime: int64(split) * interval, IntervalNs: interval, Levels: rows[split:]}
	chunks, _ := st.AddChunks([]model.Chunk{chunk0, chunk1})

	base := model.SPIParameters{
		ClockChannel: 0, DataChannel: 1, CSChannel: 2, CSActive: 0,
		BitOrder: "msb", ClockEdge: "rising", BitsPerWord: 8,
	}
	rising := base
	cfg1, _ := st.CreateConfig(model.Config{
		CaptureID: capture.ID, Protocol: "spi",
		Name: "模式0 MSB 上升沿采样（正确）", SPI: &rising,
	})
	falling := base
	falling.ClockEdge = "falling"
	cfg2, _ := st.CreateConfig(model.Config{
		CaptureID: capture.ID, Protocol: "spi",
		Name: "对照：下降沿采样（错误边沿）", SPI: &falling,
	})
	result1, _ := st.Recalculate(cfg1.ID)
	result2, _ := st.Recalculate(cfg2.ID)
	return map[string]any{
		"capture": capture, "chunks": chunks,
		"configs": []model.Config{cfg1, cfg2},
		"results": []model.Result{result1, result2},
	}
}
