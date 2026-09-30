package store

import (
	"path/filepath"
	"testing"

	"logicstation/internal/model"
)

func TestSnapshotReloadPreservesResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.json")
	first := New(path)
	capture := first.CreateCapture("spi", []string{"CLK", "MOSI", "CS"})
	_, err := first.AddChunks([]model.Chunk{{CaptureID: capture.ID, Index: 0, StartTime: 0, IntervalNs: 100, Levels: [][]uint8{{0, 0, 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	spi := model.SPIParameters{ClockChannel: 0, DataChannel: 1, CSChannel: 2, CSActive: 0, BitOrder: "msb", ClockEdge: "rising", BitsPerWord: 8}
	cfg, err := first.CreateConfig(model.Config{CaptureID: capture.ID, Protocol: "spi", SPI: &spi})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Recalculate(cfg.ID); err != nil {
		t.Fatal(err)
	}

	reloaded := New(path)
	results := reloaded.ListResults(capture.ID)
	if len(results) != 1 || results[0].ConfigID != cfg.ID || results[0].Protocol != "spi" {
		t.Fatalf("reload lost result identity: %#v", results)
	}
	if _, chunks, ok := reloaded.GetCapture(capture.ID); !ok || len(chunks) != 1 {
		t.Fatalf("reload lost capture/chunks")
	}
	configs := reloaded.ListConfigs(capture.ID)
	if len(configs) != 1 || configs[0].SPI == nil || configs[0].SPI.ClockEdge != "rising" {
		t.Fatalf("reload lost spi config: %#v", configs)
	}
}
