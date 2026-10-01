package api

import (
	"fmt"
	"sort"

	"logicstation/internal/model"
	"logicstation/internal/store"
)

type change struct {
	at    int64
	level uint8
}

func CreateDemo(st *store.Store) map[string]any {
	captures := make([]map[string]any, 0, 2)
	for _, name := range []string{"内置 UART 核对波形", "内置 SPI 核对波形"} {
		if capture, ok := st.FindCaptureByName(name); ok {
			// 示例已存在：按原采样身份重开用户保存过的配置与结果，
			// 绝不重复创建同名采样，刷新页面后仍是同一份对照。
			_, chunks, _ := st.GetCapture(capture.ID)
			configs := st.ListConfigs(capture.ID)
			results := make([]model.Result, 0, len(configs))
			for _, cfg := range configs {
				result, ok := st.GetResult(fmt.Sprintf("%s:v%d-result", cfg.ID, cfg.Version))
				if !ok {
					result, _ = st.Recalculate(cfg.ID)
				}
				results = append(results, result)
			}
			captures = append(captures, map[string]any{
				"capture": capture, "chunks": chunks, "configs": configs, "results": results,
			})
			continue
		}
		if name == "内置 UART 核对波形" {
			captures = append(captures, createUARTDemo(st))
		} else {
			captures = append(captures, createSPIDemo(st))
		}
	}
	return map[string]any{"captures": captures}
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

// createSPIDemo 生成时钟同步串行核对波形：
// 已知字节 0x48、跨块报文 0x69、片选中途结束的不完整报文，
// 以及片选无效期间的错误时钟边沿。
func createSPIDemo(st *store.Store) map[string]any {
	capture := st.CreateCapture("内置 SPI 核对波形", []string{"SCK", "MOSI", "CS"})
	const interval = int64(100)
	const half = int64(500)
	var changes [3][]change
	var cursor int64
	set := func(channel int, level uint8) {
		changes[channel] = append(changes[channel], change{cursor, level})
	}
	idle := func(ns int64) { cursor += ns }
	sendBit := func(bit uint8) {
		set(1, bit)
		idle(half)
		set(0, 1)
		idle(half)
		set(0, 0)
	}
	sendBits := func(value byte, from, to int) {
		for i := from; i >= to; i-- {
			sendBit((value >> i) & 1)
		}
	}

	set(0, 0)
	set(1, 0)
	set(2, 1)
	idle(2000)

	// 报文一：0x48，完整字节。
	set(2, 0)
	idle(half)
	sendBits(0x48, 7, 0)
	idle(half)
	set(2, 1)
	idle(2000)

	// 报文二：0x69，跨采样块续接。
	set(2, 0)
	idle(half)
	sendBits(0x69, 7, 5)
	splitTime := cursor
	sendBits(0x69, 4, 0)
	idle(half)
	set(2, 1)
	idle(2000)

	// 报文三：片选在 5 位后中途结束，应输出不完整报文。
	set(2, 0)
	idle(half)
	sendBits(0x55, 7, 3)
	set(2, 1)
	idle(2000)

	// 片选无效期间的错误时钟边沿。
	set(1, 1)
	idle(half)
	set(0, 1)
	idle(half)
	set(0, 0)
	idle(2000)

	totalSamples := int(cursor/interval) + 4
	split := int(splitTime / interval)
	levels := make([][]uint8, totalSamples)
	for i := 0; i < totalSamples; i++ {
		t := int64(i) * interval
		row := make([]uint8, 3)
		for channel := 0; channel < 3; channel++ {
			level := uint8(0)
			if channel == 2 {
				level = 1
			}
			for _, change := range changes[channel] {
				if change.at <= t+interval/2 {
					level = change.level
				}
			}
			row[channel] = level
		}
		levels[i] = row
	}
	chunks, _ := st.AddChunks([]model.Chunk{
		{CaptureID: capture.ID, Index: 0, StartTime: 0, IntervalNs: interval, Levels: levels[:split]},
		{CaptureID: capture.ID, Index: 1, StartTime: int64(split) * interval, IntervalNs: interval, Levels: levels[split:]},
	})

	base := model.SPIParameters{
		ClockChannel: 0, DataChannel: 1, CSChannel: 2, CSActive: 0,
		ClockEdge: "rising", BitOrder: "msb", BitsPerWord: 8,
	}
	lsb := base
	lsb.BitOrder = "lsb"
	falling := base
	falling.ClockEdge = "falling"
	specs := []struct {
		name   string
		params model.SPIParameters
	}{
		{"SPI MSB 上升沿", base},
		{"SPI LSB 上升沿（对照位序）", lsb},
		{"SPI MSB 下降沿（对照错误边沿）", falling},
	}
	configs := []model.Config{}
	results := []model.Result{}
	for _, spec := range specs {
		params := spec.params
		cfg, err := st.CreateConfig(model.Config{
			CaptureID: capture.ID, Protocol: "spi", Name: spec.name, SPIParameters: &params,
		})
		if err != nil {
			continue
		}
		result, err := st.Recalculate(cfg.ID)
		if err != nil {
			continue
		}
		configs = append(configs, cfg)
		results = append(results, result)
	}
	return map[string]any{"capture": capture, "chunks": chunks, "configs": configs, "results": results}
}
