import { useEffect, useMemo, useState } from 'react';
import { api } from './lib/api';
import type { NewConfigBody } from './lib/api';
import { ConfigForm } from './components/ConfigForm';
import { ResultList } from './components/ResultList';
import { WaveformTrack } from './components/WaveformTrack';
import type { CaptureBundle } from './types';

export default function App() {
  const [bundles, setBundles] = useState<CaptureBundle[]>([]);
  const [selectedCaptureId, setSelectedCaptureId] = useState<string>();
  const [selectedResultId, setSelectedResultId] = useState<string>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(true);

  const loadSaved = async (): Promise<CaptureBundle[]> => {
    const captures = await api.captures();
    const loaded: CaptureBundle[] = [];
    for (const capture of captures) {
      const [{ chunks }, configs, results] = await Promise.all([
        api.capture(capture.id),
        api.configs(capture.id),
        api.results(capture.id),
      ]);
      loaded.push({ capture, chunks, configs, results });
    }
    return loaded;
  };

  useEffect(() => {
    (async () => {
      try {
        let loaded = await loadSaved();
        if (loaded.length === 0) {
          const demo = await api.demo();
          loaded = [demo.uart, demo.spi];
        }
        setBundles(loaded);
        setSelectedCaptureId(loaded[0]?.capture.id);
        setSelectedResultId(loaded[0]?.results[loaded[0].results.length - 1]?.id);
      } catch (err) {
        setError(String(err));
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const bundle = useMemo(
    () => bundles.find((item) => item.capture.id === selectedCaptureId) ?? bundles[0],
    [bundles, selectedCaptureId],
  );

  const createConfig = async (body: NewConfigBody) => {
    if (!bundle) return;
    const { config, result } = await api.createConfig(bundle.capture.id, body);
    setBundles((current) => current.map((item) => item.capture.id === bundle.capture.id
      ? {
          ...item,
          configs: [...item.configs.filter((c) => c.id !== config.id), config],
          results: [...item.results.filter((r) => r.id !== result.id), result],
        }
      : item));
    setSelectedResultId(result.id);
  };

  const selectedResult = useMemo(
    () => bundle?.results.find((result) => result.id === selectedResultId) ?? bundle?.results[bundle.results.length - 1],
    [bundle, selectedResultId],
  );
  const selectedConfig = bundle?.configs.find((config) => config.id === selectedResult?.configId);

  if (loading || !bundle) return <main className="loading">{error ? <pre>{error}</pre> : '正在载入采样记录…'}</main>;

  return (
    <main>
      <header>
        <div>
          <p className="eyebrow">Hardware Logic Analyzer</p>
          <h1>硬件逻辑分析工作站</h1>
          <p>人工电平采样 · Go 解码 · UART / SPI 多配置对照 · 结果版本互不覆盖</p>
        </div>
        <select value={bundle.capture.id} onChange={(e) => {
          setSelectedCaptureId(e.target.value);
          const next = bundles.find((item) => item.capture.id === e.target.value);
          setSelectedResultId(next?.results[next.results.length - 1]?.id);
        }}>
          {bundles.map((item) => <option key={item.capture.id} value={item.capture.id}>{item.capture.name}</option>)}
        </select>
      </header>
      <div className="layout">
        <div className="left-column">
          <ConfigForm key={bundle.capture.id} channelCount={bundle.capture.channelNames.length} initial={selectedConfig} onSubmit={createConfig} />
          <ResultList configs={bundle.configs} results={bundle.results} selectedId={selectedResult?.id} onSelect={setSelectedResultId} />
        </div>
        <WaveformTrack chunks={bundle.chunks} channelNames={bundle.capture.channelNames} markers={selectedResult?.markers ?? []} />
      </div>
    </main>
  );
}
