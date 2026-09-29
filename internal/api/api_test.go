package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"logicstation/internal/engine"
	"logicstation/internal/model"
	"logicstation/internal/store"
)

func TestDemoDecodeCompareFlow(t *testing.T) {
	st := store.New()
	h := NewServer(st, engine.New(st))
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var r *http.Request
		if body != nil {
			b, _ := json.Marshal(body)
			r = httptest.NewRequest(method, path, bytes.NewReader(b))
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	// 1) built-in waveform
	w := do("POST", "/api/demo", map[string]any{"text": "Hi", "baudRate": 9600})
	if w.Code != 201 {
		t.Fatalf("demo status %d: %s", w.Code, w.Body.String())
	}
	var dr demoResp
	json.Unmarshal(w.Body.Bytes(), &dr)
	if len(dr.Blocks) == 0 {
		t.Fatal("demo returned no blocks")
	}

	// 2) two immutable decode config versions
	for _, baud := range []int{9600, 19200} {
		ver := "d" + itoaTest(baud)
		w = do("PUT", "/api/config/decode", model.DecodeConfig{
			Version: ver, Protocol: "uart", Channels: []string{"tx"}, BaudRate: baud, DataBits: 8})
		if w.Code != 201 {
			t.Fatalf("put %s status %d: %s", ver, w.Code, w.Body.String())
		}
	}

	decode := func(ver string) model.DecodeResult {
		w := do("POST", "/api/decode", decodeReq{SampleConfig: dr.SampleConfig.Version, DecodeConfig: ver})
		if w.Code != 200 {
			t.Fatalf("decode %s status %d: %s", ver, w.Code, w.Body.String())
		}
		var res model.DecodeResult
		json.Unmarshal(w.Body.Bytes(), &res)
		return res
	}
	r96 := decode("d9600")
	r192 := decode("d19200")
	if len(r96.Bytes) != 2 || r96.Bytes[0] != int('H') || r96.Bytes[1] != int('i') {
		t.Fatalf("9600 bytes = %v, want [72 105]", r96.Bytes)
	}
	if r96.ErrorCount != 0 {
		t.Fatalf("9600 errors = %d", r96.ErrorCount)
	}
	if r96.Version == r192.Version {
		t.Fatal("distinct decode configs must yield distinct result versions")
	}

	w = do("POST", "/api/compare", compareReq{A: r96.Version, B: r192.Version})
	var diff engine.Diff
	json.Unmarshal(w.Body.Bytes(), &diff)
	if diff.Same {
		t.Error("9600 vs 19200 results must not compare equal")
	}
}

func itoaTest(i int) string {
	if i == 0 {
		return "0"
	}
	b := []byte{}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
