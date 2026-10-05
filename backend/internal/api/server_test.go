package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	if len(captures) != 6 {
		t.Fatalf("captures after reload=%d, want 6", len(captures))
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

func TestCrossCaptureAlignmentPersistsAndClassifiesGap(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "logicstation.json")
	st, err := store.NewPersistent(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(st, "")
	demoReq := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}"))
	demoRec := httptest.NewRecorder()
	handler.ServeHTTP(demoRec, demoReq)
	if demoRec.Code != http.StatusOK {
		t.Fatalf("demo status=%d body=%s", demoRec.Code, demoRec.Body.String())
	}
	alignments := st.ListAlignments()
	if len(alignments) != 2 {
		t.Fatalf("alignments=%d, want 2", len(alignments))
	}
	align := alignments[0]
	if len(align.Anchors) != 4 || len(align.ExcludedCandidates) == 0 {
		t.Fatalf("alignment does not retain every anchor and excluded duplicate candidate: %+v", align)
	}
	body := fmt.Sprintf(`{"alignmentId":%q}`, align.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/results/cross-diff", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sampling-missing") {
		t.Fatalf("cross diff=%d body=%s", rec.Code, rec.Body.String())
	}
	reopened, err := store.NewPersistent(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := reopened.GetAlignment(align.ID)
	if !ok || len(loaded.Anchors) != 4 || len(loaded.ExcludedCandidates) != len(align.ExcludedCandidates) {
		t.Fatalf("reopened alignment lost audit data: %+v ok=%v", loaded, ok)
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

func demoResetAlignment(t *testing.T, handler http.Handler, st *store.Store) (string, string) {
	t.Helper()
	demoReq := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}"))
	demoRec := httptest.NewRecorder()
	handler.ServeHTTP(demoRec, demoReq)
	if demoRec.Code != http.StatusOK {
		t.Fatalf("demo status=%d", demoRec.Code)
	}
	for _, a := range st.ListAlignments() {
		if strings.Contains(a.Name, "时钟重置") {
			return a.ID, a.BaseResultID
		}
	}
	t.Fatalf("reset demo alignment not preset")
	return "", ""
}

// 两次时钟重置 + 重复报文的人工采样核对：跨段差异必须回到原始采集段，
// 跨段跳转不能进入错误的一轮同名时间区间。
func TestResetDemoCrossDiffTracesSegmentsAndRounds(t *testing.T) {
	st := store.New()
	handler := NewServer(st, "")
	alignmentID, _ := demoResetAlignment(t, handler, st)
	req := httptest.NewRequest(http.MethodPost, "/api/results/cross-diff", strings.NewReader(fmt.Sprintf("{\"alignmentId\":%q}", alignmentID)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cross-diff=%d body=%s", rec.Code, rec.Body.String())
	}
	var report struct {
		Diffs []struct {
			Cause              string   `json:"cause"`
			BaseSegmentIndex   int      `json:"baseSegmentIndex"`
			OtherSegmentIndex  int      `json:"otherSegmentIndex"`
			OtherNativeStartNs int64    `json:"otherNativeStartNs"`
			EvidenceAnchorIDs  []string `json:"evidenceAnchorIds"`
		} `json:"diffs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	var signal, drift bool
	for _, d := range report.Diffs {
		switch d.Cause {
		case "actual-signal":
			// C 段 1 的 C 字节 ↔ D 的 X 字节：真实协议内容变化。
			if d.BaseSegmentIndex != 1 || len(d.EvidenceAnchorIDs) == 0 {
				t.Fatalf("signal diff must trace to segment 1 with anchor evidence: %+v", d)
			}
			// 反查另一采样必须落在第二轮（约 450000），不能进入第一轮同名区间。
			if d.OtherNativeStartNs < 300000 || d.OtherNativeStartNs > 600000 {
				t.Fatalf("segment 1 jump landed in wrong round: %+v", d)
			}
			signal = true
		case "clock-drift":
			// 段 2 的 B：内容一致但残差超限，解释为漂移而非信号差异。
			if d.BaseSegmentIndex != 2 {
				t.Fatalf("drift diff must come from segment 2: %+v", d)
			}
			drift = true
		}
	}
	if !signal || !drift {
		t.Fatalf("expected actual-signal and clock-drift diffs: %s", rec.Body.String())
	}
}

// 人工确认与现有对齐相矛盾时，指出冲突经过的锚点和段，旧决定不被静默改动。
func TestAlignmentRevisionReportsConflictAndKeepsOldDecision(t *testing.T) {
	st := store.New()
	handler := NewServer(st, "")
	_, baseResultID := demoResetAlignment(t, handler, st)
	base, _ := st.GetResult(baseResultID)
	var otherID string
	for _, a := range st.ListAlignments() {
		if strings.Contains(a.Name, "时钟重置") {
			otherID = a.OtherResultID
		}
	}
	other, _ := st.GetResult(otherID)
	var baseBytes, otherBytes []string
	for _, m := range base.Markers {
		if m.Kind == "byte" {
			baseBytes = append(baseBytes, m.ID)
		}
	}
	for _, m := range other.Markers {
		if m.Kind == "byte" {
			otherBytes = append(otherBytes, m.ID)
		}
	}
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/alignments", strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	// 同一采集段对内先后关系倒序：409 并指出冲突锚点和段。
	bad := fmt.Sprintf("{\"name\":\"倒序\",\"baseResultId\":%q,\"otherResultId\":%q,\"anchors\":["+
		"{\"baseMarkerId\":%q,\"otherMarkerId\":%q},{\"baseMarkerId\":%q,\"otherMarkerId\":%q}]}",
		baseResultID, otherID, baseBytes[0], otherBytes[1], baseBytes[1], otherBytes[0])
	if rec := post(bad); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "conflicts") {
		t.Fatalf("contradictory ordering=%d body=%s", rec.Code, rec.Body.String())
	}
	// 修订某段锚点：同一基准事件连到另一对象，响应指出与旧对齐的冲突，
	// 旧对齐及其锚点保持不变。
	before := len(st.ListAlignments())
	revised := fmt.Sprintf("{\"name\":\"修订\",\"baseResultId\":%q,\"otherResultId\":%q,\"anchors\":["+
		"{\"baseMarkerId\":%q,\"otherMarkerId\":%q}]}",
		baseResultID, otherID, baseBytes[0], otherBytes[2])
	rec := post(revised)
	if rec.Code != http.StatusOK {
		t.Fatalf("revision=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "contradicts-saved-alignment") {
		t.Fatalf("revision must report conflicts with saved alignment: %s", rec.Body.String())
	}
	if len(st.ListAlignments()) != before+1 {
		t.Fatalf("revision must create a new alignment id, old one untouched")
	}

	// 跨采集段首尾逆序的修订：第一段事件对到另一采样末尾、末段事件
	// 对到其开头，必须指出跨段顺序冲突，旧对齐保持不可变。
	var seg2Base, firstOther, lastOther string
	for _, m := range base.Markers {
		if m.Kind == "byte" && m.SegmentIndex == 2 && seg2Base == "" {
			seg2Base = m.ID
		}
	}
	for _, m := range other.Markers {
		if m.Kind != "byte" {
			continue
		}
		if firstOther == "" {
			firstOther = m.ID
		}
		lastOther = m.ID
	}
	// 新锚点本身跨段逆序：直接 409，不保存。
	reversed := fmt.Sprintf("{\"name\":\"首尾逆序\",\"baseResultId\":%q,\"otherResultId\":%q,\"anchors\":["+
		"{\"baseMarkerId\":%q,\"otherMarkerId\":%q},{\"baseMarkerId\":%q,\"otherMarkerId\":%q}]}",
		baseResultID, otherID, baseBytes[0], lastOther, seg2Base, firstOther)
	if rec := post(reversed); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "cross-segment-ordering") {
		t.Fatalf("reversed cross-segment ordering=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(st.ListAlignments()) != before+1 {
		t.Fatalf("reversed ordering must not be saved or overwrite the old decision")
	}
}

// 已保存的比较可以在刷新/服务重启后通过对齐 ID 重新打开：
// 返回同一组锚点、被排除候选与差异解释。
func TestSavedAlignmentCanBeListedAndReopened(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "logicstation.json")
	st, err := store.NewPersistent(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(st, "")
	demoReq := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}"))
	demoRec := httptest.NewRecorder()
	handler.ServeHTTP(demoRec, demoReq)

	listReq := httptest.NewRequest(http.MethodGet, "/api/alignments", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list=%d", listRec.Code)
	}
	var list []struct {
		ID      string `json:"id"`
		Anchors []any  `json:"anchors"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil || len(list) < 2 {
		t.Fatalf("saved alignments=%s err=%v", listRec.Body.String(), err)
	}
	id := list[len(list)-1].ID

	// 重启后按同一 ID 取回，锚点与差异解释不变。
	reopenedStore, err := store.NewPersistent(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	reopenedHandler := NewServer(reopenedStore, "")
	getReq := httptest.NewRequest(http.MethodGet, "/api/alignments/"+id, nil)
	getRec := httptest.NewRecorder()
	reopenedHandler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), id) {
		t.Fatalf("reopen alignment=%d body=%s", getRec.Code, getRec.Body.String())
	}
	diffReq := httptest.NewRequest(http.MethodPost, "/api/results/cross-diff", strings.NewReader(fmt.Sprintf("{\"alignmentId\":%q}", id)))
	diffRec := httptest.NewRecorder()
	reopenedHandler.ServeHTTP(diffRec, diffReq)
	if diffRec.Code != http.StatusOK || !strings.Contains(diffRec.Body.String(), id) {
		t.Fatalf("reopen cross-diff=%d body=%s", diffRec.Code, diffRec.Body.String())
	}
}
