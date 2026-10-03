import { useMemo, useState } from 'react';
import { api } from '../lib/api';
import type { Alignment, CrossDiffReport, DemoCapture, Marker } from '../types';

interface Props {
  demos: DemoCapture[];
  onOpen: (captureId: string, marker: Marker) => void;
}

interface AnchorDraft { baseMarkerId: string; otherMarkerId: string; manualOrderConfirmed: boolean }

function markerLabel(marker: Marker) {
  return (marker.segmentId ?? 'segment') + ' · ' + marker.label + ' @' + (marker.startNs / 1000).toFixed(1) + 'µs';
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
  const [drafts, setDrafts] = useState<AnchorDraft[]>([{ baseMarkerId: '', otherMarkerId: '', manualOrderConfirmed: true }]);
  const [alignment, setAlignment] = useState<Alignment>();
  const [report, setReport] = useState<CrossDiffReport>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

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
      const anchors = drafts.filter((d) => d.baseMarkerId && d.otherMarkerId).map((draft) => {
        const baseMarker = base!.result.markers.find((m) => m.id === draft.baseMarkerId);
        const otherMarker = other!.result.markers.find((m) => m.id === draft.otherMarkerId);
        return {
          baseMarkerId: draft.baseMarkerId,
          otherMarkerId: draft.otherMarkerId,
          baseSegmentId: baseMarker?.segmentId,
          otherSegmentId: otherMarker?.segmentId,
          manualOrderConfirmed: draft.manualOrderConfirmed,
          evidence: draft.manualOrderConfirmed ? '用户在面板按锚点顺序确认跨段事件先后' : undefined,
        };
      });
      const saved = await api.createAlignment(baseResultId, otherResultId, anchors);
      setAlignment(saved.alignment);
      setReport(await api.crossDiff(saved.alignment.id));
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
  });

  const candidates = useMemo(() => alignment?.excludedCandidates ?? [], [alignment]);

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
          <label className="inline-check">
            <input type="checkbox" checked={draft.manualOrderConfirmed} onChange={(e) => setDrafts((items) => items.map((it, i) => i === index ? { ...it, manualOrderConfirmed: e.target.checked } : it))} />
            顺序确认
          </label>
        </div>)}
        <button onClick={() => setDrafts((items) => [...items, { baseMarkerId: '', otherMarkerId: '', manualOrderConfirmed: true }])}>增加共同事件</button>
      </div>}
      <button disabled={busy || !canAlign} onClick={align}>{busy ? '对齐中…' : '建立新的对齐比较'}</button>
      {error && <p className="error-text">{error}</p>}
      {report && <div className="cross-report">
        <p>{report.offsetSummary}；差异 {report.diffCount} 处，证据不足 {report.unmatchedCount} 处。</p>
        {report.segments.map((segment, i) => <small key={i}>分段 {i + 1}: offset={segment.offsetNs}ns, drift={segment.driftRate.toFixed(6)}</small>)}
        {!report.identical && <ul>
          {report.diffs.map((diff, i) => <li key={i} className={'cause-' + diff.cause}>
            <strong>{diff.causeText}</strong>
            <span>{diff.detail}</span>
            <small>可信半宽 ±{diff.confidenceHalfWidthNs}ns</small>
            <span>
              {diff.base && <button onClick={() => onOpen(base!.demo.capture.id, diff.base!)}>基准原始区间</button>}
              <button onClick={() => onOpen(base!.demo.capture.id, mappedMarker(diff))}>映射区间</button>
              {diff.other && <button onClick={() => onOpen(other!.demo.capture.id, diff.other!)}>另一采样原始区间</button>}
              {diff.otherNativeStartNs !== undefined && <button onClick={() => onOpen(other!.demo.capture.id, { id: 'native-range', kind: 'error', startNs: diff.otherNativeStartNs!, endNs: diff.otherNativeEndNs!, channel: diff.base?.channel ?? 0, severity: 'error', label: '基准异常的可能区间' })}>反查另一采样可能区间</button>}
            </span>
          </li>)}
        </ul>}
        <h3>锚点与排除候选</h3>
        {alignment?.anchors.map((anchor) => <details key={anchor.id} open>
          <summary>{anchor.id}: {anchor.baseSegmentId}@{anchor.baseTimeNs}ns ↔ {anchor.otherSegmentId}@{anchor.otherTimeNs}ns，±{anchor.uncertaintyNs}ns{anchor.manualOrderConfirmed ? '，顺序已人工确认' : ''}</summary>
          {anchor.evidence && <p>{anchor.evidence}</p>}
          {candidates.filter((candidate) => candidate.anchorId === anchor.id).map((candidate) => <p key={candidate.id} className={candidate.status}>
            {candidate.status === 'accepted' ? '采纳' : '排除'}：{candidate.reason}
            {candidate.residualNs !== undefined && <span>；残差 {candidate.residualNs}ns</span>}
          </p>)}
        </details>)}
      </div>}
    </section>
  );
}
