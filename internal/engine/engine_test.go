package engine

import (
	"testing"

	"logicstation/internal/demo"
	"logicstation/internal/model"
	"logicstation/internal/store"
)

func TestRecomputeVersionsAndCompare(t *testing.T) {
	st := store.New()
	sampleCfg := model.SampleConfig{Version: "s1", Channels: []string{"tx"}, PeriodNs: 1000}
	if err := st.PutSampleConfig(sampleCfg); err != nil {
		t.Fatal(err)
	}
	for _, b := range demo.Generate(sampleCfg, demo.Params{Channel: "tx", BaudRate: 9600, Bytes: []byte("OK")}) {
		if err := st.AppendBlock(b); err != nil {
			t.Fatal(err)
		}
	}
	c1 := model.DecodeConfig{Version: "d-9600", Protocol: "uart", Channels: []string{"tx"},
		BaudRate: 9600, DataBits: 8, Parity: "none", StopBits: 1}
	c2 := c1
	c2.Version = "d-19200"
	c2.BaudRate = 19200
	if err := st.PutDecodeConfig(c1); err != nil {
		t.Fatal(err)
	}
	if err := st.PutDecodeConfig(c2); err != nil {
		t.Fatal(err)
	}

	eng := New(st)
	r1, err := eng.Recompute("s1", "d-9600")
	if err != nil {
		t.Fatal(err)
	}
	r1Again, err := eng.Recompute("s1", "d-9600")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Version != r1Again.Version {
		t.Fatal("recompute version not stable")
	}
	r2, err := eng.Recompute("s1", "d-19200")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Version == r1.Version {
		t.Fatal("different decode config must yield a different result version")
	}
	if len(r1.Bytes) != 2 || r1.Bytes[0] != int('O') || r1.Bytes[1] != int('K') {
		t.Fatalf("r1 bytes = %v", r1.Bytes)
	}
	if r1.ErrorCount != 0 {
		t.Fatalf("r1 errors = %d, want 0", r1.ErrorCount)
	}
	diff, err := eng.Compare(r1.Version, r2.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Same {
		t.Error("9600 vs 19200 results must differ")
	}
	same, err := eng.Compare(r1.Version, r1.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !same.Same {
		t.Error("result must compare equal to itself")
	}
}
