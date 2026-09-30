package api

import (
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

func TestDemoEndpointIncludesSPIComparison(t *testing.T) {
	handler := NewServer(store.New(), "")
	request := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{"0xA5", "0x3C", "incomplete-frame", "片选中途结束", "对照：下降沿采样"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("spi demo response missing %q: %s", expected, body)
		}
	}
}
