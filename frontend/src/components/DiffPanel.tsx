import { useState } from 'react';
import { api } from '../lib/api';
import type { Marker, Result, ResultDiff } from '../types';

interface Props {
  results: Result[];
  onLocate: (marker: Marker) => void;
}

const causeNames: Record<ResultDiff['cause'], string> = {
  'bit-order': '位序不同',
  'clock-edge': '采样边沿不同',
  'incomplete-fragment': '片段不完整',
  other: '其他参数差异',
};

export function DiffPanel({ results, onLocate }: Props) {
  const [aId, setAId] = useState<string>();
  const [bId, setBId] = useState<string>();
  const [diff, setDiff] = useState<ResultDiff | null>();
  const [message, setMessage] = useState<string>();
  const [busy, setBusy] = useState(false);

  const compare = async () => {
    if (!aId || !bId) return;
    setBusy(true);
    try {
      const response = await api.diff(aId, bId);
      setDiff(response.diff);
      setMessage(response.message);
      if (response.diff) {
        onLocate({
          id: `diff-${response.diff.index}`,
          kind: 'error',
          startNs: response.diff.startNs,
          endNs: response.diff.endNs,
          channel: response.diff.channel,
          label: `差异 #${response.diff.index + 1}`,
          severity: 'error',
        });
      }
    } finally {
      setBusy(false);
    }
  };

  if (results.length < 2) return null;
  const options = results.map((result) => (
    <option key={result.id} value={result.id}>v{result.configVersion}（{result.validBytes} 字节 / {result.errorCount} 错误）</option>
  ));

  return (
    <section className="panel">
      <h2>版本差异定位</h2>
      <div className="diff-picker">
        <label>版本 A<select value={aId ?? ''} onChange={(e) => setAId(e.target.value)}><option value="" disabled>选择结果</option>{options}</select></label>
        <label>版本 B<select value={bId ?? ''} onChange={(e) => setBId(e.target.value)}><option value="" disabled>选择结果</option>{options}</select></label>
        <button disabled={busy || !aId || !bId || aId === bId} onClick={compare}>{busy ? '对比中…' : '定位首个差异'}</button>
      </div>
      {message && <p className="diff-message">{message}</p>}
      {diff && (
        <div className="diff-result">
          <strong>第 {diff.index + 1} 个字节 · {causeNames[diff.cause]}</strong>
          <p>{diff.explanation}</p>
          <p>
            A：{diff.a ? diff.a.label : '（无对应字节）'}
            {diff.a?.rawBits ? ` [${diff.a.rawBits}]` : ''}
            {'　'}B：{diff.b ? diff.b.label : '（无对应字节）'}
            {diff.b?.rawBits ? ` [${diff.b.rawBits}]` : ''}
          </p>
        </div>
      )}
    </section>
  );
}
