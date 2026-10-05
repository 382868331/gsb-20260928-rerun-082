import { useEffect, useMemo, useState } from 'react';
import { api } from '../lib/api';
import type { Alignment, AlignmentConflict, CrossDiffReport, DemoCapture, Marker } from '../types';

interface Props {
  demos: DemoCapture[];
  onOpen: (captureId: string, marker: Marker) => void;
}

interface AnchorDraft { baseMarkerId: string; otherMarkerId: string }

function markerLabel(marker: Marker) {
  return '段' + (marker.segmentIndex + 1) + ' · ' + marker.label + ' @' + (marker.startNs / 1000).toFixed(1) + 'µs';
}

function resultOf(demos: DemoCapture[], resultId?: string) {
  for (const demo of demos) {
    const result = demo.results.find((item) => item.id === resultId);
    if (result) return { demo, result };
  }
  return undefined;
}

export function CrossAlignmentPanel({ demos, onOpen }: Props) {
  const [baseResultId, setBaseResultId] = useState<string>();
  const [otherResultId, setOtherResultId] = useState<string>();
  const [drafts, setDrafts] = useState<AnchorDraft[]>([{ baseMarkerId: '', otherMarkerId: '' }]);
  const [alignment, setAlignment] = useState<Alignment>();
  const [report, setReport] = useState<CrossDiffReport>();
  const [conflicts, setConflicts] = useState<AlignmentConflict[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [savedComparisons, setSavedComparisons] = useState<Alignment[]>([]);

  // 已保存的比较（锚点、被排除候选与差异解释）持久化在服务端；
  // 刷新页面或服务重启后仍可从同一对齐 ID 返回同一组采集段，
  // 不会跳入另一轮同名时间区间。
  const refreshSaved = async () => {
    try {
      setSavedComparisons(await api.alignments());
    } catch (err) {
      setError(String(err));
    }
  };

  useEffect(() => { void refreshSaved(); }, []);

  const base = resultOf(demos, baseResultId);
  const other = resultOf(demos, otherResultId);
  const canAlign = Boolean(base && other && base.demo.capture.id !== other.demo.capture.id && drafts.every((d) => d.baseMarkerId && d.otherMarkerId));
  const resultOptions = (excludeId?: string) => demos.flatMap((demo) => demo.results
    .filter((result) => result.id !== excludeId)
    .map((result) => ({ demo, result })));

  const align = async () => {
    if (!baseResultId || !otherResultId) return;
    setBusy(true);
    setError(undefined);
    try {
      const saved = await api.createAlignment(baseResultId, otherResultId, drafts.filter((d) => d.baseMarkerId && d.otherMarkerId));
      setAlignment(saved.alignment);
      setConflicts(saved.conflicts ?? []);
      setReport(await api.crossDiff(saved.alignment.id));
      await refreshSaved();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };

  const mappedMarker = (diff: CrossDiffReport['diffs'][number]): Marker => ({
    id: 'cross-mapped',
    kind: 'error',
    startNs: diff.mappedStartNs ?? diff.base?.startNs ?? 0,
    endNs: diff.mappedEndNs ?? diff.base?.endNs ?? 0,
    channel: diff.base?.channel ?? diff.other?.channel ?? 0,
    severity: 'error',
    label: diff.causeText,
    segmentIndex: Math.max(0, diff.baseSegmentIndex),
  });

  const candidates = useMemo(() => alignment?.excludedCandidates ?? [], [alignment]);

  // 重开一份已保存比较：恢复两端结果版本、原始锚点选择与差异解释，
  // 映射按持久化的锚点重算，因此仍指向同一组采集段和同一轮重复报文。
  const reopen = async (item: Alignment) => {
    setBusy(true);
    setError(undefined);
    try {
      const full = await api.alignment(item.id);
      setBaseResultId(full.baseResultId);
      setOtherResultId(full.otherResultId);
      setDrafts(full.anchors.map((anchor) => ({ baseMarkerId: anchor.baseMarkerId, otherMarkerId: anchor.otherMarkerId })));
      setConflicts([]);
      setAlignment(full);
      setReport(await api.crossDiff(full.id));
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="panel cross-panel">
      <h2>跨采样共同事件对齐</h2>
      <p className="hint">选择重复脉冲等共同事件。锚点只用于新比较，不会改写采样或已保存解码版本。</p>
      <label>基准采样/版本
        <select value={baseResultId ?? ''} onChange={(e) => { setBaseResultId(e.target.value); setAlignment(undefined); setReport(undefined); }}>
          <option value="" disabled>选择基准结果…</option>
          {resultOptions(otherResultId).map(({ demo, result }) => <option key={result.id} value={result.id}>{demo.capture.name} · v{result.configVersion}</option>)}
        </select>
      </label>
      <label>另一采样/版本
        <select value={otherResultId ?? ''} onChange={(e) => { setOtherResultId(e.target.value); setAlignment(undefined); setReport(undefined); }}>
          <option value="" disabled>选择另一结果…</option>
          {resultOptions(baseResultId).map(({ demo, result }) => <option key={result.id} value={result.id}>{demo.capture.name} · v{result.configVersion}</option>)}
        </select>
      </label>
      {base && other && <div className="anchor-editor">
        {drafts.map((draft, index) => <div className="anchor-row" key={index}>
          <select value={draft.baseMarkerId} onChange={(e) => setDrafts((items) => items.map((it, i) => i === index ? { ...it, baseMarkerId: e.target.value } : it))}>
            <option value="" disabled>基准事件…</option>
            {base.result.markers.filter((m) => m.kind === 'byte').map((m) => <option key={m.id} value={m.id}>{markerLabel(m)}</option>)}
          </select>
          <span>↔</span>
          <select value={draft.otherMarkerId} onChange={(e) => setDrafts((items) => items.map((it, i) => i === index ? { ...it, otherMarkerId: e.target.value } : it))}>
            <option value="" disabled>另一事件…</option>
            {other.result.markers.filter((m) => m.kind === 'byte').map((m) => <option key={m.id} value={m.id}>{markerLabel(m)}</option>)}
          </select>
        </div>)}
        <button onClick={() => setDrafts((items) => [...items, { baseMarkerId: '', otherMarkerId: '' }])}>增加共同事件</button>
      </div>}
      <button disabled={busy || !canAlign} onClick={align}>{busy ? '对齐中…' : '建立新的对齐比较'}</button>
      {savedComparisons.length > 0 && <div className="saved-comparisons">
        <h3>已保存的比较（刷新/重启后可重开）</h3>
        {savedComparisons.map((item) => <button key={item.id} className="saved-comparison" disabled={busy} onClick={() => reopen(item)}>
          {item.name} · 锚点 {item.anchors.length} 个 · {new Date(item.createdAt).toLocaleString()}
        </button>)}
      </div>}
      {error && <p className="error-text">{error}</p>}
      {conflicts.length > 0 && <div className="conflict-list">
        <h3>与既有对齐的冲突（旧决定保持不变）</h3>
        {conflicts.map((conflict, i) => <p key={i}>
          {conflict.message}
          <small>（段：基准 {conflict.baseSegmentIndex + 1} ↔ 另一 {conflict.otherSegmentIndex + 1}；锚点 {conflict.previousAnchorId} → {conflict.newAnchorId}）</small>
        </p>)}
      </div>}
      {report && <div className="cross-report">
        <p>{report.offsetSummary}；差异 {report.diffCount} 处，时钟漂移 {report.driftCount} 处，证据不足 {report.unmatchedCount} 处。</p>
        {report.segments.map((segment, i) => <small key={i}>段对（基准段 {segment.baseSegmentIndex + 1} ↔ 另一段 {segment.otherSegmentIndex + 1}）分段 {i + 1}: offset={segment.offsetNs}ns, drift={segment.driftRate.toFixed(6)}</small>)}
        {!report.identical && <ul>
          {report.diffs.map((diff, i) => <li key={i} className={'cause-' + diff.cause}>
            <strong>{diff.causeText}</strong>
            <span>{diff.detail}</span>
            <small>
              采集段：基准 {diff.baseSegmentIndex >= 0 ? diff.baseSegmentIndex + 1 : '—'} ↔ 另一 {diff.otherSegmentIndex >= 0 ? diff.otherSegmentIndex + 1 : '—'}
              ；可信半宽 ±{diff.confidenceHalfWidthNs}ns
              {diff.evidenceAnchorIds && diff.evidenceAnchorIds.length > 0 && '；对齐证据：' + diff.evidenceAnchorIds.join(', ')}
            </small>
            <span>
              {diff.base && <button onClick={() => onOpen(base!.demo.capture.id, diff.base!)}>基准原始区间</button>}
              <button onClick={() => onOpen(base!.demo.capture.id, mappedMarker(diff))}>映射区间</button>
              {diff.other && <button onClick={() => onOpen(other!.demo.capture.id, diff.other!)}>另一采样原始区间</button>}
              {diff.otherNativeStartNs !== undefined && <button onClick={() => onOpen(other!.demo.capture.id, { id: 'native-range', kind: 'error', startNs: diff.otherNativeStartNs!, endNs: diff.otherNativeEndNs!, channel: diff.base?.channel ?? 0, severity: 'error', label: '基准异常的可能区间（段 ' + (diff.otherSegmentIndex + 1) + '）', segmentIndex: Math.max(0, diff.otherSegmentIndex) })}>反查另一采样可能区间</button>}
            </span>
          </li>)}
        </ul>}
        <h3>锚点与排除候选</h3>
        {alignment?.anchors.map((anchor) => <details key={anchor.id} open>
          <summary>{anchor.id}: 段{anchor.baseSegmentIndex + 1} {anchor.baseTimeNs}ns ↔ 段{anchor.otherSegmentIndex + 1} {anchor.otherTimeNs}ns，±{anchor.uncertaintyNs}ns</summary>
          {candidates.filter((candidate) => candidate.anchorId === anchor.id).map((candidate) => <p key={candidate.id} className={candidate.status}>
            {candidate.status === 'accepted' ? '采纳' : '排除'}：{candidate.reason}
            {candidate.residualNs !== undefined && <span>；残差 {candidate.residualNs}ns</span>}
          </p>)}
        </details>)}
      </div>}
    </section>
  );
}
