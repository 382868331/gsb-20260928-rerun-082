package api

import (
	"encoding/json"
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

func TestDiffEndpointClassifiesBitOrderAndEdge(t *testing.T) {
	st := store.New()
	handler := NewServer(st, "")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}")))
	if response.Code != http.StatusOK {
		t.Fatalf("demo status=%d", response.Code)
	}
	var demo struct {
		Captures []struct {
			Capture struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"capture"`
			Results []struct {
				ID       string `json:"id"`
				Protocol string `json:"protocol"`
			} `json:"results"`
		} `json:"captures"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &demo); err != nil {
		t.Fatal(err)
	}
	var spiResults []string
	for _, capture := range demo.Captures {
		if !strings.Contains(capture.Capture.Name, "SPI") {
			continue
		}
		for _, result := range capture.Results {
			spiResults = append(spiResults, result.ID)
		}
	}
	if len(spiResults) != 3 {
		t.Fatalf("expected 3 SPI results, got %d", len(spiResults))
	}

	diff := func(a, b string) map[string]any {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/results/diff?a="+a+"&b="+b, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("diff status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}

	// MSB 与 LSB 对照：首个差异应归因于位序。
	payload := diff(spiResults[0], spiResults[1])
	entry, ok := payload["diff"].(map[string]any)
	if !ok {
		t.Fatalf("expected diff, got %v", payload)
	}
	if entry["cause"] != "bit-order" {
		t.Fatalf("cause=%v, want bit-order", entry["cause"])
	}

	// MSB 与错误边沿对照：首个差异应归因于采样边沿。
	payload = diff(spiResults[0], spiResults[2])
	entry, ok = payload["diff"].(map[string]any)
	if !ok {
		t.Fatalf("expected diff, got %v", payload)
	}
	if entry["cause"] != "clock-edge" {
		t.Fatalf("cause=%v, want clock-edge", entry["cause"])
	}

	// 相同结果对比：无差异。
	payload = diff(spiResults[0], spiResults[0])
	if payload["diff"] != nil {
		t.Fatalf("expected nil diff, got %v", payload["diff"])
	}

	// 未知结果 ID 返回 404。
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/results/diff?a=nope&b="+spiResults[0], nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", recorder.Code)
	}
}
