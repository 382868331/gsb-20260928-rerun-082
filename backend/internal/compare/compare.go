// Package compare 对同一采样的两个解码结果版本做差异定位：
// 找到首个不同字节/标记的波形区间，并说明差异来自位序、采样边沿还是片段不完整。
package compare

import (
	"fmt"

	"logicstation/internal/model"
)

// Cause 描述差异根因类别。
const (
	CauseBitOrder   = "bit-order"
	CauseClockEdge  = "clock-edge"
	CauseIncomplete = "incomplete-frame"
	CauseOther      = "other-decode-difference"
)

// MarkerDiff 是按位置对齐后的单个标记差异。
type MarkerDiff struct {
	Index     int           `json:"index"`
	StartNs   int64         `json:"startNs"`
	EndNs     int64         `json:"endNs"`
	Channel   int           `json:"channel"`
	Base      *model.Marker `json:"base,omitempty"`
	Other     *model.Marker `json:"other,omitempty"`
	Cause     string        `json:"cause"`
	CauseText string        `json:"causeText"`
	Detail    string        `json:"detail"`
}

// Report 是两个结果版本的对照报告。
type Report struct {
	BaseResultID  string       `json:"baseResultId"`
	OtherResultID string       `json:"otherResultId"`
	Identical     bool         `json:"identical"`
	DiffCount     int          `json:"diffCount"`
	FirstDiff     *MarkerDiff  `json:"firstDiff,omitempty"`
	Diffs         []MarkerDiff `json:"diffs"`
}

// ConfigSnapshot 仅暴露判断根因所需的配置字段。
type ConfigSnapshot struct {
	Protocol  string
	ClockEdge string
	BitOrder  string
}

func kindKey(m *model.Marker) string {
	if m == nil {
		return ""
	}
	if m.Kind == "error" {
		return "error:" + m.Code
	}
	return "byte"
}

// classify 根据两个标记及生成它们的配置判断差异根因。
func classify(base, other *model.Marker, baseCfg, otherCfg ConfigSnapshot) (string, string, string) {
	bIncomplete := base != nil && base.Code == "incomplete-frame"
	oIncomplete := other != nil && other.Code == "incomplete-frame"
	if bIncomplete || oIncomplete {
		detail := "一个版本在此区间得到完整字节，另一个版本标记为片选中断的残段"
		if bIncomplete && oIncomplete {
			detail = "两个版本都只收到不完整残段，残段内容/长度不同"
		}
		return CauseIncomplete, "片段不完整：片选在字节中途结束，残段不能补成完整字节", detail
	}
	// 同一段原始波形、同样的原始采样位但组装出的值不同，只能来自位序解释差异。
	if base != nil && other != nil && base.RawBits != "" && base.RawBits == other.RawBits && kindKey(base) == kindKey(other) {
		return CauseBitOrder, "位序不同：同一组原始位按 MSB/LSB 解释成不同字节",
			fmt.Sprintf("原始位均为 %s，MSB/LSB 组装结果不同", base.RawBits)
	}
	// 配置的采样边沿不同，且区间上的原始位/取值不同：采样沿偏移半拍。
	if baseCfg.ClockEdge != "" && otherCfg.ClockEdge != "" && baseCfg.ClockEdge != otherCfg.ClockEdge {
		return CauseClockEdge, "采样边沿不同：另一版本在相反时钟沿采样，数据相对边沿偏移半拍",
			fmt.Sprintf("%s 沿 vs %s 沿", edgeText(baseCfg.ClockEdge), edgeText(otherCfg.ClockEdge))
	}
	if baseCfg.BitOrder != "" && otherCfg.BitOrder != "" && baseCfg.BitOrder != otherCfg.BitOrder {
		return CauseBitOrder, "位序不同：MSB/LSB 解释导致首个字节不同",
			fmt.Sprintf("%s vs %s", baseCfg.BitOrder, otherCfg.BitOrder)
	}
	return CauseOther, "解码内容不同：协议参数或波形状态导致该区间解释不一致", describe(base, other)
}

func edgeText(edge string) string {
	if edge == "rising" {
		return "上升"
	}
	if edge == "falling" {
		return "下降"
	}
	return edge
}

func describe(base, other *model.Marker) string {
	switch {
	case base == nil:
		return "基准版本在该位置无标记，对照版本多出: " + other.Label
	case other == nil:
		return "对照版本在该位置无标记，基准版本为: " + base.Label
	default:
		return fmt.Sprintf("基准 %q，对照 %q", base.Label, other.Label)
	}
}

// Results 对齐两个结果的标记序列并逐项比较。标记按解码顺序对应同一波形位置；
// 长度不一致时尾部按缺失处理。
func Results(base, other model.Result, baseCfg, otherCfg model.Config) Report {
	bs := snapshot(baseCfg)
	os := snapshot(otherCfg)
	n := len(base.Markers)
	if len(other.Markers) > n {
		n = len(other.Markers)
	}
	report := Report{BaseResultID: base.ID, OtherResultID: other.ID, Diffs: []MarkerDiff{}}
	for i := 0; i < n; i++ {
		var bm, om *model.Marker
		if i < len(base.Markers) {
			bm = &base.Markers[i]
		}
		if i < len(other.Markers) {
			om = &other.Markers[i]
		}
		if equal(bm, om) {
			continue
		}
		cause, text, detail := classify(bm, om, bs, os)
		diff := MarkerDiff{Index: i, Base: bm, Other: om, Cause: cause, CauseText: text, Detail: detail}
		reference := bm
		if reference == nil {
			reference = om
		}
		diff.StartNs = reference.StartNs
		diff.EndNs = reference.EndNs
		diff.Channel = reference.Channel
		report.Diffs = append(report.Diffs, diff)
	}
	report.DiffCount = len(report.Diffs)
	report.Identical = report.DiffCount == 0
	if report.DiffCount > 0 {
		first := report.Diffs[0]
		report.FirstDiff = &first
	}
	return report
}

func snapshot(cfg model.Config) ConfigSnapshot {
	s := ConfigSnapshot{Protocol: cfg.Protocol}
	if cfg.SPIParameters != nil {
		s.ClockEdge = cfg.SPIParameters.ClockEdge
		s.BitOrder = cfg.SPIParameters.BitOrder
	}
	return s
}

// equal 认为同一波形区间、同类型、同值、同原始位、同错误码即相同。
func equal(base, other *model.Marker) bool {
	if base == nil || other == nil {
		return false
	}
	return base.StartNs == other.StartNs && base.EndNs == other.EndNs &&
		base.Kind == other.Kind && base.Code == other.Code &&
		base.RawBits == other.RawBits &&
		((base.Value == nil && other.Value == nil) ||
			(base.Value != nil && other.Value != nil && *base.Value == *other.Value))
}
