package api

import (
	"fmt"
	"sort"

	"logicstation/internal/alignment"
	"logicstation/internal/model"
	"logicstation/internal/store"
)

type change struct {
	at    int64
	level uint8
}

func CreateDemo(st *store.Store) map[string]any {
	captures := make([]map[string]any, 0, 2)
	for _, name := range []string{"内置 UART 核对波形", "内置 SPI 核对波形", "跨采样人工波形 A", "跨采样人工波形 B"} {
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
		switch name {
		case "内置 UART 核对波形":
			captures = append(captures, createUARTDemo(st))
		case "内置 SPI 核对波形":
			captures = append(captures, createSPIDemo(st))
		case "跨采样人工波形 A":
			captures = append(captures, createCrossCapture(st, true))
		default:
			captures = append(captures, createCrossCapture(st, false))
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

func createCrossCapture(st *store.Store, first bool) map[string]any {
	nameA, nameB := "跨采样人工波形 A", "跨采样人工波形 B"
	if captureA, okA := st.FindCaptureByName(nameA); okA {
		if captureB, okB := st.FindCaptureByName(nameB); okB {
			if first {
				_, chunksA, _ := st.GetCapture(captureA.ID)
				return demoPayload(st, captureA, chunksA)
			}
			_, chunksB, _ := st.GetCapture(captureB.ID)
			return demoPayload(st, captureB, chunksB)
		}
	}
	captureA, ok := st.FindCaptureByName(nameA)
	if !ok {
		captureA = st.CreateCapture(nameA, []string{"CLK-REF", "RX"})
	}
	captureB := st.CreateCapture(nameB, []string{"CLK-REF", "RX"})

	baud := 100000
	interval := int64(2500)
	bit := int64(1000000000 / baud)
	values := []byte{'A', 'B', 'A', 'C'}
	startsA := []int64{20000, 150000, 280000, 410000}
	startsB := []int64{50000, 180000, 300000, 400000}

	build := func(capture model.Capture, starts []int64, split bool) []model.Chunk {
		changes := []change{{at: 0, level: 1}}
		for i, value := range values {
			cursor := starts[i]
			changes = append(changes, change{at: cursor, level: 0})
			for bitIndex := 0; bitIndex < 8; bitIndex++ {
				changes = append(changes, change{at: cursor + bit*int64(bitIndex+1), level: (value >> bitIndex) & 1})
			}
			changes = append(changes, change{at: cursor + bit*9, level: 1})
		}
		end := starts[len(starts)-1] + bit*10 + interval*2
		rows := func(from, to int64) [][]uint8 {
			out := [][]uint8{}
			for t := from; t <= to; t += interval {
				level := uint8(1)
				for _, c := range changes {
					if c.at <= t+interval/2 {
						level = c.level
					}
				}
				out = append(out, []uint8{0, level})
			}
			return out
		}
		if !split {
			return []model.Chunk{{CaptureID: capture.ID, Index: 0, StartTime: 0, IntervalNs: interval, Levels: rows(0, end)}}
		}
		// 第二帧结束后刻意保留 20 µs 空白，解码器只能标记 sampling-gap，不能补造波形。
		return []model.Chunk{
			{CaptureID: capture.ID, Index: 0, StartTime: 0, IntervalNs: interval, Levels: rows(0, 260000)},
			{CaptureID: capture.ID, Index: 1, StartTime: 280000, IntervalNs: interval, Levels: rows(280000, end)},
		}
	}
	chunksA, _ := st.AddChunks(build(captureA, startsA, true))
	chunksB, _ := st.AddChunks(build(captureB, startsB, false))

	makeConfig := func(capture model.Capture) (model.Config, model.Result) {
		cfg, _ := st.CreateConfig(model.Config{CaptureID: capture.ID, Protocol: "uart", Name: "100k 8N1 跨采样核对", Parameters: model.UARTParameters{Channel: 1, Baud: baud, DataBits: 8, Parity: "none", StopBits: 1, IdleLevel: 1, ClockTolerance: 0.04}})
		result, _ := st.Recalculate(cfg.ID)
		return cfg, result
	}
	_, resultA := makeConfig(captureA)
	_, resultB := makeConfig(captureB)
	byteMarkers := func(result model.Result) []model.Marker {
		out := []model.Marker{}
		for _, marker := range result.Markers {
			if marker.Kind == "byte" {
				out = append(out, marker)
			}
		}
		return out
	}
	bytesA, bytesB := byteMarkers(resultA), byteMarkers(resultB)
	anchors := []model.AlignmentAnchor{}
	for i := 0; i < 4; i++ {
		anchors = append(anchors, model.AlignmentAnchor{ID: fmt.Sprintf("anchor-%d", i+1), BaseMarkerID: bytesA[i].ID, OtherMarkerID: bytesB[i].ID, BaseTimeNs: bytesA[i].StartNs, OtherTimeNs: bytesB[i].StartNs, UncertaintyNs: interval/2 + 1})
	}
	align := model.Alignment{Name: "重复脉冲/缺段/漂移变化核对", BaseCaptureID: captureA.ID, OtherCaptureID: captureB.ID, BaseResultID: resultA.ID, OtherResultID: resultB.ID, Anchors: anchors, Status: alignment.StatusReady}
	for _, a := range align.Anchors {
		bm := findCrossMarker(resultA.Markers, a.BaseMarkerID)
		for _, om := range resultB.Markers {
			if om.ID != a.OtherMarkerID && sameCrossSignature(bm, om) {
				align.ExcludedCandidates = append(align.ExcludedCandidates, model.AlignmentCandidate{ID: a.ID + "-excluded", AnchorID: a.ID, BaseMarkerID: bm.ID, OtherMarkerID: om.ID, BaseTimeNs: bm.StartNs, OtherTimeNs: om.StartNs, Status: alignment.CandidateExcluded, Reason: "重复 A 脉冲候选：时间锚点明确排除另一个同值字节"})
			}
		}
	}
	_, _ = st.SaveAlignment(align)
	if first {
		return demoPayload(st, captureA, chunksA)
	}
	return demoPayload(st, captureB, chunksB)
}

func demoPayload(st *store.Store, capture model.Capture, chunks []model.Chunk) map[string]any {
	configs := st.ListConfigs(capture.ID)
	results := make([]model.Result, 0, len(configs))
	for _, cfg := range configs {
		result, _ := st.Recalculate(cfg.ID)
		results = append(results, result)
	}
	return map[string]any{"capture": capture, "chunks": chunks, "configs": configs, "results": results}
}

func findCrossMarker(markers []model.Marker, id string) model.Marker {
	for _, m := range markers {
		if m.ID == id {
			return m
		}
	}
	return model.Marker{}
}

func sameCrossSignature(a, b model.Marker) bool {
	return a.Kind == b.Kind && a.Code == b.Code && ((a.Value == nil && b.Value == nil) || (a.Value != nil && b.Value != nil && *a.Value == *b.Value))
}
