import { useState } from 'react';
import { api } from '../lib/api';
import type { Config, DiffReport, Marker, MarkerDiff, Result } from '../types';

interface Props {
  configs: Config[];
  results: Result[];
  selectedResultId?: string;
  onLocate: (marker: Marker) => void;
}

function versionLabel(result: Result, configs: Config[]) {
  const config = configs.find((item) => item.id === result.configId);
  return `v${result.configVersion} ${config?.name ?? result.protocol}`;
}

function diffToMarker(diff: MarkerDiff, suffix: string): Marker {
  return {
    id: `diff-${diff.index}`,
    kind: 'error',
    startNs: diff.startNs,
    endNs: diff.endNs,
    channel: diff.channel,
    severity: 'error',
    label: suffix,
  };
}

export function DiffPanel({ configs, results, selectedResultId, onLocate }: Props) {
  const [baseId, setBaseId] = useState<string>();
  const [otherId, setOtherId] = useState<string | undefined>(selectedResultId);
  const [report, setReport] = useState<DiffReport>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  if (results.length < 2) return null;

  const compare = async () => {
    if (!baseId || !otherId || baseId === otherId) return;
    setBusy(true);
    setError(undefined);
    try {
      setReport(await api.diff(baseId, otherId));
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="panel diff-panel">
      <h2>版本差异定位</h2>
      <label>基准版本
        <select value={baseId ?? ''} onChange={(e) => setBaseId(e.target.value)}>
          <option value="" disabled>选择基准…</option>
          {results.map((result) => <option key={result.id} value={result.id}>{versionLabel(result, configs)}</option>)}
        </select>
      </label>
      <label>对照版本
        <select value={otherId ?? ''} onChange={(e) => setOtherId(e.target.value)}>
          <option value="" disabled>选择对照…</option>
          {results.map((result) => <option key={result.id} value={result.id}>{versionLabel(result, configs)}</option>)}
        </select>
      </label>
      <button disabled={busy || !baseId || !otherId || baseId === otherId} onClick={compare}>
        {busy ? '对照中…' : '定位两个版本差异'}
      </button>
      {error && <p className="error-text">{error}</p>}
      {report && (
        <div className="diff-report">
          {report.identical ? (
            <p>两个版本解码结果完全一致，无差异。</p>
          ) : (
            <>
              <p>共 {report.diffCount} 处差异。{report.firstDiff?.causeText}</p>
              {report.firstDiff && (
                <button
                  className="locate-diff"
                  onClick={() => onLocate(diffToMarker(report.firstDiff!, report.firstDiff!.causeText))}
                >
                  跳到首个差异波形区间（第 {report.firstDiff.index + 1} 个标记）
                </button>
              )}
              <ul>
                {report.diffs.map((diff) => (
                  <li key={diff.index}>
                    <button onClick={() => onLocate(diffToMarker(diff, diff.causeText))}>
                      <strong>#{diff.index + 1} {diff.causeText}</strong>
                      <span>{diff.detail}</span>
                      <small>基准：{diff.base?.label ?? '（无标记）'} ｜ 对照：{diff.other?.label ?? '（无标记）'}</small>
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}
    </section>
  );
}
