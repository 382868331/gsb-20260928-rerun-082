import { useCallback, useEffect, useMemo, useState } from 'react';
import { api } from '../lib/api';
import type {
  Alignment,
  AlignmentAnchor,
  AlignmentEstimate,
  AnchorCandidate,
  CrossDiff,
  CrossReport,
  DemoCapture,
  Marker,
} from '../types';

interface Props {
  demos: DemoCapture[];
  onLocateCross: (captureId: string, resultId: string | undefined, marker: Marker) => void;
}

function formatNs(ns: number) {
  if (Math.abs(ns) >= 1_000_000) return `${(ns / 1_000_000).toFixed(3)} ms`;
  if (Math.abs(ns) >= 1_000) return `${(ns / 1_000).toFixed(2)} µs`;
  return `${ns} ns`;
}

function diffMarker(diff: CrossDiff, side: 'base' | 'other'): Marker | undefined {
  const source = side === 'base' ? diff.base : diff.other;
  const start = side === 'base' ? diff.baseStartNs : diff.otherStartNs;
  const end = side === 'base' ? diff.baseEndNs : diff.otherEndNs;
  if (start === undefined || end === undefined) return undefined;
  return {
    id: `cross-${diff.index}-${side}`,
    kind: 'error',
    startNs: start,
    endNs: Math.max(end, start + 1),
    channel: source?.channel ?? 0,
    severity: 'error',
    label: diff.causeText,
  };
}

export function AlignmentPanel({ demos, onLocateCross }: Props) {
  const [baseCaptureId, setBaseCaptureId] = useState<string>();
  const [otherCaptureId, setOtherCaptureId] = useState<string>();
  const [groups, setGroups] = useState<Alignment[]>([]);
  const [groupId, setGroupId] = useState<string>();
  const [versions, setVersions] = useState<Alignment[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<number>();
  const [estimate, setEstimate] = useState<AlignmentEstimate>();
  const [draft, setDraft] = useState<AlignmentAnchor[]>([]);
  const [baseResultId, setBaseResultId] = useState<string>();
  const [otherResultId, setOtherResultId] = useState<string>();
  const [candidates, setCandidates] = useState<{ accepted: AnchorCandidate[]; excluded: AnchorCandidate[] }>();
  const [report, setReport] = useState<CrossReport>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (demos.length === 0) return;
    const alignA = demos.find((demo) => demo.capture.name.includes('对齐 A'));
    const alignB = demos.find((demo) => demo.capture.name.includes('对齐 B'));
    setBaseCaptureId((current) => current ?? (alignA ?? demos[0]).capture.id);
    setOtherCaptureId((current) => current ?? (alignB ?? demos[demos.length - 1]).capture.id);
  }, [demos]);

  const refreshGroups = useCallback(async () => {
    try {
      setGroups(await api.alignments());
    } catch (err) {
      setError(String(err));
    }
  }, []);

  useEffect(() => { void refreshGroups(); }, [refreshGroups]);

  const matchingGroups = useMemo(
    () => groups.filter((group) => group.baseCaptureId === baseCaptureId && group.otherCaptureId === otherCaptureId),
    [groups, baseCaptureId, otherCaptureId],
  );

  useEffect(() => {
    setGroupId(matchingGroups[0]?.groupId);
    setReport(undefined);
    setCandidates(undefined);
  }, [matchingGroups]);

  useEffect(() => {
    if (!groupId) {
      setVersions([]);
      setEstimate(undefined);
      setDraft([]);
      return;
    }
    void (async () => {
      try {
        const detail = await api.alignment(groupId);
        setVersions(detail.versions);
        setSelectedVersion(detail.alignment.version);
        setEstimate(detail.estimate);
        setDraft(detail.alignment.anchors);
      } catch (err) {
        setError(String(err));
      }
    })();
  }, [groupId]);

  useEffect(() => {
    const version = versions.find((item) => item.version === selectedVersion);
    if (!version || !groupId) return;
    setDraft(version.anchors);
    void (async () => {
      try {
        const detail = await api.alignment(groupId, version.version);
        setEstimate(detail.estimate);
      } catch (err) {
        setError(String(err));
      }
    })();
  }, [selectedVersion, versions, groupId]);

  const baseDemo = demos.find((demo) => demo.capture.id === baseCaptureId);
  const otherDemo = demos.find((demo) => demo.capture.id === otherCaptureId);
  const baseResult = baseDemo?.results.find((result) => result.id === baseResultId) ?? baseDemo?.results[0];
  const otherResult = otherDemo?.results.find((result) => result.id === otherResultId) ?? otherDemo?.results[0];

  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setError(undefined);
    try {
      await action();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };

  const createGroup = () => run(async () => {
    if (!baseCaptureId || !otherCaptureId || baseCaptureId === otherCaptureId) return;
    const created = await api.createAlignment('跨采样对齐', baseCaptureId, otherCaptureId, []);
    await refreshGroups();
    setGroupId(created.groupId);
  });

  const loadCandidates = () => run(async () => {
    if (!groupId || !baseResult || !otherResult) return;
    setCandidates(await api.alignmentCandidates(groupId, baseResult.id, otherResult.id));
  });

  const saveVersion = () => run(async () => {
    if (!groupId) return;
    const saved = await api.addAlignmentVersion(groupId, draft);
    const detail = await api.alignment(groupId);
    setVersions(detail.versions);
    setSelectedVersion(saved.alignment.version);
    setEstimate(saved.estimate);
    setReport(undefined);
  });

  const compare = () => run(async () => {
    if (!groupId || !selectedVersion || !baseResult || !otherResult) return;
    setReport(await api.compareAlignment(groupId, selectedVersion, baseResult.id, otherResult.id));
  });

  const addCandidateAnchor = (candidate: AnchorCandidate) => {
    setDraft((current) => [
      ...current,
      {
        id: `auto-${candidate.baseMarkerId}`,
        label: `共同字节 0x${candidate.value.toString(16).toUpperCase().padStart(2, '0')}`,
        baseNs: candidate.baseNs,
        otherNs: candidate.otherNs,
        source: 'auto',
      },
    ]);
  };

  if (demos.length < 2) return null;

  return (
    <section className="panel alignment-panel">
      <h2>跨采样对齐与对照</h2>
      <div className="align-row">
        <label>基准采样
          <select value={baseCaptureId ?? ''} onChange={(e) => setBaseCaptureId(e.target.value)}>
            {demos.map((demo) => <option key={demo.capture.id} value={demo.capture.id}>{demo.capture.name}</option>)}
          </select>
        </label>
        <label>对照采样
          <select value={otherCaptureId ?? ''} onChange={(e) => setOtherCaptureId(e.target.value)}>
            {demos.map((demo) => <option key={demo.capture.id} value={demo.capture.id}>{demo.capture.name}</option>)}
          </select>
        </label>
        {matchingGroups.length === 0 ? (
          <button disabled={busy || !baseCaptureId || !otherCaptureId || baseCaptureId === otherCaptureId} onClick={createGroup}>
            新建对齐组
          </button>
        ) : (
          <label>对齐组
            <select value={groupId ?? ''} onChange={(e) => setGroupId(e.target.value)}>
              {matchingGroups.map((group) => <option key={group.groupId} value={group.groupId}>{group.name}</option>)}
            </select>
          </label>
        )}
        {versions.length > 0 && (
          <label>对齐依据版本
            <select value={selectedVersion ?? ''} onChange={(e) => setSelectedVersion(Number(e.target.value))}>
              {versions.map((version) => (
                <option key={version.id} value={version.version}>
                  v{version.version}（{version.anchors.length} 个锚点）
                </option>
              ))}
            </select>
          </label>
        )}
      </div>
      {error && <p className="error-text">{error}</p>}
      {groupId && (
        <>
          <h3>对齐锚点（共同事件）</h3>
          <table className="anchor-table">
            <thead>
              <tr><th>事件</th><th>基准时间</th><th>对照时间</th><th>偏移</th><th>残差</th><th /></tr>
            </thead>
            <tbody>
              {draft.map((anchor) => {
                const offset = estimate?.anchors.find((item) => item.anchorId === anchor.id);
                return (
                  <tr key={anchor.id}>
                    <td>{anchor.label ?? anchor.id}{anchor.source === 'auto' ? '（自动）' : ''}</td>
                    <td>{formatNs(anchor.baseNs)}</td>
                    <td>{formatNs(anchor.otherNs)}</td>
                    <td>{formatNs(anchor.otherNs - anchor.baseNs)}</td>
                    <td>{offset ? formatNs(offset.residualNs) : '—'}</td>
                    <td>
                      <button onClick={() => setDraft((current) => current.filter((item) => item.id !== anchor.id))}>
                        移除
                      </button>
                    </td>
                  </tr>
                );
              })}
              {draft.length === 0 && <tr><td colSpan={6}>尚无锚点，请先发现候选或新建对齐。</td></tr>}
            </tbody>
          </table>
          {estimate && estimate.anchorCount > 0 && (
            <p className="estimate-line">
              偏移 {formatNs(estimate.offsetNs)}（观测范围 {formatNs(estimate.minOffsetNs)} ~ {formatNs(estimate.maxOffsetNs)}）
              {estimate.hasDrift
                ? ` · 漂移 ${estimate.driftPpm.toFixed(1)} ppm · 可信范围 ±${formatNs(estimate.confidenceNs)}`
                : ' · 单锚点仅估计纯偏移，无法估计漂移'}
              {estimate.anchorCount > 1 && ` · 覆盖 ${formatNs(estimate.coverageStartNs)} ~ ${formatNs(estimate.coverageEndNs)}，范围外片段保持无法对应`}
            </p>
          )}
          <div className="align-row">
            <label>基准结果
              <select value={baseResult?.id ?? ''} onChange={(e) => setBaseResultId(e.target.value)}>
                {baseDemo?.results.map((result) => <option key={result.id} value={result.id}>v{result.configVersion} {result.protocol}</option>)}
              </select>
            </label>
            <label>对照结果
              <select value={otherResult?.id ?? ''} onChange={(e) => setOtherResultId(e.target.value)}>
                {otherDemo?.results.map((result) => <option key={result.id} value={result.id}>v{result.configVersion} {result.protocol}</option>)}
              </select>
            </label>
            <button disabled={busy || !baseResult || !otherResult} onClick={loadCandidates}>发现共同事件候选</button>
            <button disabled={busy || draft.length === 0} onClick={saveVersion}>将对齐依据存为新版本</button>
            <button disabled={busy || draft.length === 0 || !baseResult || !otherResult} onClick={compare}>
              按当前版本对照解码差异
            </button>
          </div>
          {candidates && (
            <div className="candidates">
              <h3>候选共同事件（{candidates.accepted.length} 个可用）</h3>
              <ul>
                {candidates.accepted.map((candidate) => (
                  <li key={candidate.baseMarkerId}>
                    <button onClick={() => addCandidateAnchor(candidate)}>
                      采纳 0x{candidate.value.toString(16).toUpperCase().padStart(2, '0')}（偏移 {formatNs(candidate.offsetNs)}）
                    </button>
                  </li>
                ))}
              </ul>
              <h3>被排除的候选（{candidates.excluded.length} 个，保留原因供核对）</h3>
              <ul>
                {candidates.excluded.map((candidate, index) => (
                  <li key={`${candidate.baseMarkerId}-${index}`} className="excluded">
                    0x{candidate.value.toString(16).toUpperCase().padStart(2, '0')}：{candidate.reason}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {report && (
            <div className="cross-report">
              <h3>
                对照结果（对齐 v{report.version}）：{report.matched} 个标记一致，{report.diffCount} 处差异/无法对应
              </h3>
              <ul>
                {report.diffs.map((diff) => {
                  const baseMarker = diffMarker(diff, 'base');
                  const otherMarker = diffMarker(diff, 'other');
                  return (
                    <li key={diff.index} className={`cause-${diff.cause}`}>
                      <strong>{diff.causeText}</strong>
                      <span>{diff.detail}</span>
                      <small>
                        基准：{diff.base?.label ?? '（无标记）'} ｜ 对照：{diff.other?.label ?? '（无标记）'}
                        {diff.confidenceLowNs !== undefined && ` · 对照区间可信范围 ${formatNs(diff.confidenceLowNs)} ~ ${formatNs(diff.confidenceHighNs!)}`}
                      </small>
                      <span className="locate-buttons">
                        {baseMarker && baseDemo && (
                          <button onClick={() => onLocateCross(baseDemo.capture.id, baseResult?.id, baseMarker)}>
                            看基准区间
                          </button>
                        )}
                        {otherMarker && otherDemo && (
                          <button onClick={() => onLocateCross(otherDemo.capture.id, otherResult?.id, otherMarker)}>
                            看对照可能区间
                          </button>
                        )}
                      </span>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
        </>
      )}
    </section>
  );
}
