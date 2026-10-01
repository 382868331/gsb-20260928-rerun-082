package decode

import (
	"strings"
	"testing"

	"logicstation/internal/model"
)

func byteMarker(value byte, start, end int64) model.Marker {
	return model.Marker{Kind: "byte", Severity: "info", Channel: 1, StartNs: start, EndNs: end, Value: &value}
}

func TestDiffResultsIdentical(t *testing.T) {
	a := model.Result{Markers: []model.Marker{byteMarker(0x48, 0, 100)}}
	b := model.Result{Markers: []model.Marker{byteMarker(0x48, 0, 100)}}
	if diff := DiffResults(a, b, model.Config{}, model.Config{}); diff != nil {
		t.Fatalf("expected nil diff, got %+v", diff)
	}
}

func TestDiffResultsIncompleteFragment(t *testing.T) {
	a := model.Result{Markers: []model.Marker{
		byteMarker(0x48, 0, 100),
		byteMarker(0x69, 200, 300),
	}}
	b := model.Result{Markers: []model.Marker{
		byteMarker(0x48, 0, 100),
		{Kind: "error", Code: "incomplete-frame", Severity: "error", Channel: 1, StartNs: 200, EndNs: 260, RawBits: "10101"},
	}}
	diff := DiffResults(a, b, model.Config{}, model.Config{})
	if diff == nil {
		t.Fatal("expected diff")
	}
	if diff.Cause != "incomplete-fragment" {
		t.Fatalf("cause=%q, want incomplete-fragment", diff.Cause)
	}
	if diff.Index != 1 || diff.StartNs != 200 {
		t.Fatalf("unexpected location: %+v", diff)
	}
	if diff.B == nil || diff.B.Code != "incomplete-frame" {
		t.Fatalf("expected fragment marker on side B: %+v", diff.B)
	}
	if !strings.Contains(diff.Explanation, "片段不完整") {
		t.Fatalf("explanation=%q", diff.Explanation)
	}
}

func TestDiffResultsBitOrder(t *testing.T) {
	a := model.Result{Markers: []model.Marker{byteMarker(0x48, 0, 100)}}
	b := model.Result{Markers: []model.Marker{byteMarker(0x12, 0, 100)}}
	msb := &model.SPIParameters{BitOrder: "msb", ClockEdge: "rising"}
	lsb := &model.SPIParameters{BitOrder: "lsb", ClockEdge: "rising"}
	diff := DiffResults(a, b, model.Config{SPIParameters: msb}, model.Config{SPIParameters: lsb})
	if diff == nil || diff.Cause != "bit-order" {
		t.Fatalf("expected bit-order diff, got %+v", diff)
	}
}
