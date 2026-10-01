package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"logicstation/internal/store"
)

func TestDemoEndpointDecodesBuiltInWaveform(t *testing.T) {
	handler := NewServer(store.New(), "")
	request := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{"0x41", "framing-error", "停止位不是空闲电平"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("demo response missing %q: %s", expected, body)
		}
	}
}

func TestDemoIsIdempotentAndReopensSavedComparisons(t *testing.T) {
	st := store.New()
	handler := NewServer(st, "")
	call := func() string {
		request := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}"))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		return response.Body.String()
	}
	first := call()
	second := call()
	// 再次载入示例必须复用同一采样身份，而不是造同名新示例。
	if strings.Count(second, "内置 UART 核对波形") != 1 || strings.Count(second, "内置 SPI 核对波形") != 1 {
		t.Fatalf("demo must list each capture once: %s", second)
	}
	captures := st.ListCaptures()
	if len(captures) != 2 {
		t.Fatalf("captures after reload=%d, want 2", len(captures))
	}
	if !strings.Contains(first, captures[0].ID) || !strings.Contains(second, captures[0].ID) {
		t.Fatalf("reload must keep original capture identity")
	}
	// 用户在 SPI 采样上另存的对照版本在重开后仍然存在。
	spi := captures[1]
	if got := len(st.ListConfigs(spi.ID)); got != 3 {
		t.Fatalf("spi configs after reload=%d, want 3", got)
	}
	if got := len(st.ListResults(spi.ID)); got != 3 {
		t.Fatalf("spi results after reload=%d, want 3", got)
	}
}

func TestDiffEndpointLocatesFirstDifference(t *testing.T) {
	st := store.New()
	handler := NewServer(st, "")
	demoReq := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}"))
	demoRec := httptest.NewRecorder()
	handler.ServeHTTP(demoRec, demoReq)
	var demo struct {
		Captures []struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
		} `json:"captures"`
	}
	if err := json.Unmarshal(demoRec.Body.Bytes(), &demo); err != nil {
		t.Fatal(err)
	}
	spiResults := demo.Captures[1].Results
	if len(spiResults) != 3 {
		t.Fatalf("want 3 spi results, got %d", len(spiResults))
	}
	body := fmt.Sprintf(`{"baseResultId":%q,"otherResultId":%q}`, spiResults[0].ID, spiResults[1].ID)
	req := httptest.NewRequest(http.MethodPost, "/api/results/diff", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	for _, expected := range []string{"firstDiff", "bit-order", "位序不同"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("diff response missing %q: %s", expected, out)
		}
	}

	// 不同采样的结果不能对照。
	uartID := demo.Captures[0].Results[0].ID
	badReq := httptest.NewRequest(http.MethodPost, "/api/results/diff",
		strings.NewReader(fmt.Sprintf(`{"baseResultId":%q,"otherResultId":%q}`, uartID, spiResults[0].ID)))
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("cross-capture diff status=%d, want 400", badRec.Code)
	}
}
