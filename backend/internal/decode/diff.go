package decode

import (
	"fmt"

	"logicstation/internal/model"
)

func byteMarkers(result model.Result) []model.Marker {
	var out []model.Marker
	for _, marker := range result.Markers {
		if marker.Kind == "byte" {
			out = append(out, marker)
		}
	}
	return out
}

func overlaps(marker model.Marker, start, end int64) bool {
	return marker.EndNs >= start && marker.StartNs <= end
}

// findFragmentError 返回覆盖指定区间的不完整片段错误标记（若存在）。
func findFragmentError(result model.Result, start, end int64) *model.Marker {
	for i := range result.Markers {
		marker := result.Markers[i]
		if marker.Kind != "error" {
			continue
		}
		if marker.Code == "incomplete-frame" || marker.Code == "sampling-gap" {
			if overlaps(marker, start, end) {
				return &result.Markers[i]
			}
		}
	}
	return nil
}

// DiffResults 定位两个结果版本的首个不同字节，并判断差异来源。
// 两个结果相同时返回 nil。
func DiffResults(a, b model.Result, cfgA, cfgB model.Config) *model.ResultDiff {
	bytesA, bytesB := byteMarkers(a), byteMarkers(b)
	index := -1
	shared := len(bytesA)
	if len(bytesB) < shared {
		shared = len(bytesB)
	}
	for i := 0; i < shared; i++ {
		va, vb := byte(0), byte(0)
		if bytesA[i].Value != nil {
			va = *bytesA[i].Value
		}
		if bytesB[i].Value != nil {
			vb = *bytesB[i].Value
		}
		if va != vb || bytesA[i].RawBits != bytesB[i].RawBits {
			index = i
			break
		}
	}
	if index < 0 && len(bytesA) != len(bytesB) {
		index = shared
	}
	if index < 0 {
		return nil
	}

	diff := &model.ResultDiff{Index: index}
	var markerA, markerB *model.Marker
	if index < len(bytesA) {
		markerA = &bytesA[index]
	}
	if index < len(bytesB) {
		markerB = &bytesB[index]
	}
	anchor := markerA
	if anchor == nil {
		anchor = markerB
	}
	if anchor == nil {
		return nil
	}
	diff.StartNs, diff.EndNs, diff.Channel = anchor.StartNs, anchor.EndNs, anchor.Channel
	if markerA != nil && markerB != nil {
		if markerB.StartNs < diff.StartNs {
			diff.StartNs = markerB.StartNs
		}
		if markerB.EndNs > diff.EndNs {
			diff.EndNs = markerB.EndNs
		}
	}
	diff.A, diff.B = markerA, markerB

	// 优先判断是否由片段不完整导致：任一侧在该区间存在
	// 不完整报文/采样间隙错误，而另一侧解出了正常字节。
	fragA := findFragmentError(a, diff.StartNs, diff.EndNs)
	fragB := findFragmentError(b, diff.StartNs, diff.EndNs)
	if fragA != nil || fragB != nil {
		diff.Cause = "incomplete-fragment"
		diff.Explanation = fmt.Sprintf("第 %d 个字节差异来自片段不完整：片选中途结束或采样间隙留下残段，按错误标记保留原始位，未补成完整字节。", index+1)
		if fragA != nil {
			diff.A = fragA
		}
		if fragB != nil {
			diff.B = fragB
		}
		return diff
	}

	spiA, spiB := cfgA.SPIParameters, cfgB.SPIParameters
	switch {
	case spiA != nil && spiB != nil && spiA.BitOrder != spiB.BitOrder:
		diff.Cause = "bit-order"
		diff.Explanation = fmt.Sprintf("第 %d 个字节差异来自位序：一侧按 %s、另一侧按 %s 组装相同的采样位。", index+1, spiA.BitOrder, spiB.BitOrder)
	case spiA != nil && spiB != nil && spiA.ClockEdge != spiB.ClockEdge:
		diff.Cause = "clock-edge"
		diff.Explanation = fmt.Sprintf("第 %d 个字节差异来自采样边沿：一侧在%s沿、另一侧在%s沿采样，采到的位流不同。", index+1, edgeName(spiA.ClockEdge), edgeName(spiB.ClockEdge))
	default:
		diff.Cause = "other"
		diff.Explanation = fmt.Sprintf("第 %d 个字节差异来自解码参数不同（如波特率、数据位或每字位数），请对照两个配置版本。", index+1)
	}
	return diff
}

func edgeName(edge string) string {
	if edge == "falling" {
		return "下降"
	}
	return "上升"
}
