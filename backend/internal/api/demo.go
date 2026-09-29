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
	return map[string]any{"capture": capture, "chunks": chunks, "config": cfg, "result": result}
}
