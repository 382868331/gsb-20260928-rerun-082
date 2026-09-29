import type { Config, Result } from '../types';

function formatNs(ns: number) {
  return `${(ns / 1000).toFixed(2)} µs`;
}

export function ResultList({ configs, results, selectedId, onSelect }: { configs: Config[]; results: Result[]; selectedId?: string; onSelect: (id: string) => void }) {
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
        <div key={marker.id} className={`marker-row ${marker.severity}`}>
          <code>{formatNs(marker.startNs)} – {formatNs(marker.endNs)}</code>
          <span>{marker.label}</span>
          {marker.rawBits && <small>{marker.rawBits}</small>}
        </div>
      ))}
    </section>
  );
}
