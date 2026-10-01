package compare

import (
	"testing"

	"logicstation/internal/model"
)

func byteMarker(id string, start, end int64, value byte, raw string) model.Marker {
	v := value
	return model.Marker{ID: id, Kind: "byte", Severity: "info", Channel: 1, StartNs: start, EndNs: end, Value: &v, RawBits: raw, Label: "byte"}
}

func spiCfg(edge, order string) model.Config {
	return model.Config{Protocol: "spi", SPIParameters: &model.SPIParameters{ClockEdge: edge, BitOrder: order}}
}

func TestBitOrderSameRawBits(t *testing.T) {
	base := model.Result{Markers: []model.Marker{byteMarker("a", 0, 100, 0x48, "01001000")}}
	other := model.Result{Markers: []model.Marker{byteMarker("b", 0, 100, 0x12, "01001000")}}
	report := Results(base, other, spiCfg("rising", "msb"), spiCfg("rising", "lsb"))
	if report.Identical || report.FirstDiff == nil {
		t.Fatalf("expected one diff: %+v", report)
	}
	if report.FirstDiff.Cause != CauseBitOrder {
		t.Fatalf("cause=%s want bit-order", report.FirstDiff.Cause)
	}
}

func TestClockEdgeDifferentRawBits(t *testing.T) {
	base := model.Result{Markers: []model.Marker{byteMarker("a", 0, 100, 0x48, "01001000")}}
	other := model.Result{Markers: []model.Marker{byteMarker("b", 0, 100, 0x69, "01101001")}}
	report := Results(base, other, spiCfg("rising", "msb"), spiCfg("falling", "msb"))
	if report.FirstDiff.Cause != CauseClockEdge {
		t.Fatalf("cause=%s want clock-edge", report.FirstDiff.Cause)
	}
}

func TestIncompleteFrame(t *testing.T) {
	base := model.Result{Markers: []model.Marker{byteMarker("a", 0, 100, 0x48, "01001")}}
	other := model.Result{Markers: []model.Marker{model.Marker{ID: "b", Kind: "error", Code: "incomplete-frame", Severity: "error", Channel: 1, StartNs: 0, EndNs: 100, RawBits: "01001", Label: "残段"}}}
	report := Results(base, other, spiCfg("rising", "msb"), spiCfg("rising", "msb"))
	if report.FirstDiff.Cause != CauseIncomplete {
		t.Fatalf("cause=%s want incomplete-frame", report.FirstDiff.Cause)
	}
}

func TestIdentical(t *testing.T) {
	m := []model.Marker{byteMarker("a", 0, 100, 0x48, "01001000")}
	report := Results(model.Result{Markers: m}, model.Result{Markers: m}, spiCfg("rising", "msb"), spiCfg("rising", "msb"))
	if !report.Identical {
		t.Fatalf("expected identical report: %+v", report)
	}
}
