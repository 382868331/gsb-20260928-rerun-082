import type { Config, Marker, Result } from '../types';

function formatNs(ns: number) {
  return `${(ns / 1000).toFixed(2)} µs`;
}

interface Props {
  configs: Config[];
  results: Result[];
  selectedId?: string;
  onSelect: (id: string) => void;
  onLocate: (marker: Marker) => void;
}

export function ResultList({ configs, results, selectedId, onSelect, onLocate }: Props) {
  return (
    <section className="panel">
      <h2>结果版本</h2>
      <div className="result-list">
        {results.map((result) => {
          const config = configs.find((item) => item.id === result.configId);
          return (
            <button key={result.id} className={selectedId === result.id ? 'selected' : ''} onClick={() => onSelect(result.id)}>
              <strong>v{result.configVersion} {config?.name ?? result.protocol}</strong>
              <span>{result.validBytes} 个有效字节 · {result.errorCount} 个错误</span>
            </button>
          );
        })}
      </div>
      {results.find((result) => result.id === selectedId)?.markers.map((marker) => (
        <button key={marker.id} className={`marker-row ${marker.severity}`} onClick={() => onLocate(marker)} title="定位到原始采样区间">
          <code>{formatNs(marker.startNs)} – {formatNs(marker.endNs)}</code>
          <span>{marker.label}</span>
          {marker.rawBits && <small>{marker.rawBits}</small>}
        </button>
      ))}
    </section>
  );
}
