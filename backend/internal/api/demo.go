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
			captures = append(captures, reopenCapture(st, capture))
			continue
		}
		if name == "内置 UART 核对波形" {
			captures = append(captures, createUARTDemo(st))
		} else {
			captures = append(captures, createSPIDemo(st))
		}
	}
	// 跨采样对齐核对对：重复字节（重复脉冲）、局部缺段、漂移变化。
	// 重开时按原采样身份复用，已保存的对齐锚点版本与被排除候选保持可追溯。
	if capture, ok := st.FindCaptureByName(alignDemoNameA); ok {
		captures = append(captures, reopenCapture(st, capture))
		if other, ok := st.FindCaptureByName(alignDemoNameB); ok {
			captures = append(captures, reopenCapture(st, other))
		}
	} else {
		captures = append(captures, createAlignDemo(st)...)
	}
	return map[string]any{"captures": captures}
}

// reopenCapture 按原采样身份重开用户保存过的配置与结果，
// 绝不重复创建同名采样，刷新页面后仍是同一份对照。
func reopenCapture(st *store.Store, capture model.Capture) map[string]any {
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
	return map[string]any{
		"capture": capture, "chunks": chunks, "configs": configs, "results": results,
	}
}

const (
	alignDemoNameA = "内置 跨采样对齐 A（基准）"
	alignDemoNameB = "内置 跨采样对齐 B（漂移缺段）"
)

// createAlignDemo 生成两份记录同一 UART 通信的人工采样：
// A 为基准；B 起点偏移、采样率不同，中段漂移率改变，
// 局部缺失一段采样（采样缺段），且末字节真实信号不同（'O' vs 'P'）。
// 序列中重复的字节 'L','L' 使自动候选无法唯一对应，会被排除并记录原因。
func createAlignDemo(st *store.Store) []map[string]any {
	const baud = 9600
	bit := int64(1000000000 / baud)
	bytesA := []byte{'H', 'E', 'L', 'L', 'O', '0', '0'}
	bytesB := []byte{'H', 'E', 'L', 'L', 'P', '0', '0'}

	changes := func(bytes []byte) []change {
		out := []change{}
		for i, value := range bytes {
			start := int64(i)*12*bit + 2*bit
			out = append(out, change{start, 0})
			for b := 0; b < 8; b++ {
				out = append(out, change{start + bit*int64(b+1), (value >> b) & 1})
			}
			out = append(out, change{start + bit*9, 1})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].at < out[j].at })
		return out
	}
	levelAt := func(changes []change, t int64) uint8 {
		level := uint8(1)
		for _, c := range changes {
			if c.at <= t {
				level = c.level
			}
		}
		return level
	}
	totalIdeal := int64(len(bytesA))*12*bit + 4*bit

	// A：均匀采样，单块。
	captureA := st.CreateCapture(alignDemoNameA, []string{"RX"})
	intervalA := bit / 4
	countA := int(totalIdeal/intervalA) + 4
	levelsA := make([][]uint8, countA)
	ca := changes(bytesA)
	for i := 0; i < countA; i++ {
		levelsA[i] = []uint8{levelAt(ca, int64(i)*intervalA)}
	}
	chunksA, _ := st.AddChunks([]model.Chunk{
		{CaptureID: captureA.ID, Index: 0, StartTime: 0, IntervalNs: intervalA, Levels: levelsA},
	})

	// B：起点偏移 + 分段漂移 + 局部缺段。
	captureB := st.CreateCapture(alignDemoNameB, []string{"RX"})
	const offset = int64(3_700_000)
	tMid := int64(3) * 12 * bit
	const d1 = 0.0002
	const d2 = -0.0002
	var forward func(int64) int64
	forward = func(t int64) int64 {
		if t <= tMid {
			return offset + int64(float64(t)*(1+d1))
		}
		return forward(tMid) + int64(float64(t-tMid)*(1+d2))
	}
	backward := func(tb int64) int64 {
		mid := forward(tMid)
		if tb <= mid {
			return int64(float64(tb-offset) / (1 + d1))
		}
		return tMid + int64(float64(tb-mid)/(1+d2))
	}
	intervalB := bit / 5
	gapStartIdeal := int64(2)*12*bit + 4*bit
	gapEndIdeal := int64(2)*12*bit + 10*bit
	gapStartB, gapEndB := forward(gapStartIdeal), forward(gapEndIdeal)
	cb := changes(bytesB)
	countB := int(forward(totalIdeal)/intervalB) + 4
	var levelsB0, levelsB1 [][]uint8
	for i := 0; i < countB; i++ {
		tb := int64(i) * intervalB
		if tb >= gapStartB && tb < gapEndB {
			continue // 局部缺失：该段没有采样，不补造电平
		}
		row := []uint8{levelAt(cb, backward(tb))}
		if tb < gapStartB {
			levelsB0 = append(levelsB0, row)
		} else {
			levelsB1 = append(levelsB1, row)
		}
	}
	chunksB, _ := st.AddChunks([]model.Chunk{
		{CaptureID: captureB.ID, Index: 0, StartTime: 0, IntervalNs: intervalB, Levels: levelsB0},
		{CaptureID: captureB.ID, Index: 1, StartTime: gapEndB, IntervalNs: intervalB, Levels: levelsB1},
	})

	entry := func(capture model.Capture, chunks []model.Chunk) map[string]any {
		cfg, _ := st.CreateConfig(model.Config{
			CaptureID: capture.ID,
			Protocol:  "uart",
			Name:      "9600 8N1 RX",
			Parameters: model.UARTParameters{
				Channel: 0, Baud: baud, DataBits: 8, Parity: "none", StopBits: 1, IdleLevel: 1, ClockTolerance: 0.03,
			},
		})
		result, _ := st.Recalculate(cfg.ID)
		return map[string]any{"capture": capture, "chunks": chunks, "configs": []model.Config{cfg}, "results": []model.Result{result}}
	}

	// 预置一组对齐依据：首字节与末字节起点两个共同事件。
	// 中段漂移变化会以锚点残差形式呈现在可信范围中，用户可增加中间锚点收敛。
	st.CreateAlignment("跨采样对齐（首/末字节起点）", captureA.ID, captureB.ID, []model.AlignmentAnchor{
		{Label: "字节 'H' 起点", BaseNs: 2 * bit, OtherNs: forward(2 * bit), Source: "manual"},
		{Label: "末字节起点", BaseNs: 6*12*bit + 2*bit, OtherNs: forward(6*12*bit + 2*bit), Source: "manual"},
	})
	return []map[string]any{entry(captureA, chunksA), entry(captureB, chunksB)}
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
