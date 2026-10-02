package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"logicstation/internal/model"
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
	for _, name := range []string{"内置 UART 核对波形", "内置 SPI 核对波形", alignDemoNameA, alignDemoNameB} {
		if strings.Count(second, name) != 1 {
			t.Fatalf("demo must list %q once: %s", name, second)
		}
	}
	captures := st.ListCaptures()
	if len(captures) != 4 {
		t.Fatalf("captures after reload=%d, want 4", len(captures))
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

func post(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	return rec
}

func TestAlignmentLifecycleAndCrossCaptureCompare(t *testing.T) {
	st := store.New()
	handler := NewServer(st, "")
	post(t, handler, "/api/demo", "{}")

	captures := st.ListCaptures()
	var base, other model.Capture
	for _, capture := range captures {
		switch capture.Name {
		case alignDemoNameA:
			base = capture
		case alignDemoNameB:
			other = capture
		}
	}
	if base.ID == "" || other.ID == "" {
		t.Fatal("demo 应包含跨采样对齐核对对")
	}
	baseResult := st.ListResults(base.ID)[0]
	otherResult := st.ListResults(other.ID)[0]

	// 预置对齐组存在；锚点与版本在重开后可追溯。
	groups := st.ListAlignments()
	if len(groups) != 1 || groups[0].BaseCaptureID != base.ID || groups[0].OtherCaptureID != other.ID {
		t.Fatalf("alignments=%+v", groups)
	}
	groupID := groups[0].GroupID

	// 自动候选：重复字节 'L' 被排除并记录原因，唯一字节被接受。
	rec := post(t, handler, "/api/alignments/"+groupID+"/candidates",
		fmt.Sprintf(`{"baseResultId":%q,"otherResultId":%q}`, baseResult.ID, otherResult.ID))
	var candidates struct {
		Accepted []struct{ Value byte } `json:"accepted"`
		Excluded []struct {
			Value  byte
			Reason string
		} `json:"excluded"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates.Accepted) == 0 || len(candidates.Excluded) == 0 {
		t.Fatalf("candidates=%+v", candidates)
	}
	foundRepeat := false
	for _, excluded := range candidates.Excluded {
		if excluded.Value == '0' && strings.Contains(excluded.Reason, "重复") {
			foundRepeat = true
		}
	}
	if !foundRepeat {
		t.Fatalf("重复脉冲候选应被排除并说明原因: %+v", candidates.Excluded)
	}

	// 跨采样对照：区分采样缺段与真实信号差异，并给出映射区间。
	rec = post(t, handler, "/api/alignments/"+groupID+"/compare",
		fmt.Sprintf(`{"baseResultId":%q,"otherResultId":%q}`, baseResult.ID, otherResult.ID))
	report := rec.Body.String()
	for _, expected := range []string{"sampling-gap", "signal-difference", "confidenceLowNs", "estimate"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("compare response missing %q: %s", expected, report)
		}
	}

	// 跨采样结果不能混用：用不属于对齐组的结果比较必须拒绝。
	uartResult := st.ListResults(captures[0].ID)[0]
	badReq := httptest.NewRequest(http.MethodPost, "/api/alignments/"+groupID+"/compare",
		strings.NewReader(fmt.Sprintf(`{"baseResultId":%q,"otherResultId":%q}`, uartResult.ID, otherResult.ID)))
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("mixed-capture compare status=%d, want 400", badRec.Code)
	}

	// 修改对齐依据生成新版本，旧版本保留；原解码结果不变。
	rec = post(t, handler, "/api/alignments/"+groupID+"/versions",
		`{"anchors":[{"label":"H","baseNs":0,"otherNs":3700000}]}`)
	var versioned struct {
		Alignment model.Alignment `json:"alignment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &versioned); err != nil {
		t.Fatal(err)
	}
	if versioned.Alignment.Version != 2 {
		t.Fatalf("version=%d, want 2", versioned.Alignment.Version)
	}
	versions, ok := st.AlignmentVersions(groupID)
	if !ok || len(versions) != 2 || len(versions[0].Anchors) != 2 {
		t.Fatalf("versions=%+v ok=%v", versions, ok)
	}
	if got := len(st.ListResults(base.ID)); got != 1 {
		t.Fatalf("修改对齐依据不应改变已保存解码结果, results=%d", got)
	}

	// 重开示例后对齐组与全部锚点版本仍可追溯。
	post(t, handler, "/api/demo", "{}")
	versions, ok = st.AlignmentVersions(groupID)
	if !ok || len(versions) != 2 {
		t.Fatalf("重开后对齐版本丢失: %+v", versions)
	}
	if len(st.ListAlignments()) != 1 {
		t.Fatal("重开不应重复创建对齐组")
	}
}
