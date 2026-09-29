import { useEffect, useMemo, useState } from 'react';
import { api } from './lib/api';
import { ConfigForm } from './components/ConfigForm';
import { ResultList } from './components/ResultList';
import { WaveformTrack } from './components/WaveformTrack';
import type { Capture, Chunk, Config, Result, UARTParameters } from './types';

export default function App() {
  const [capture, setCapture] = useState<Capture | null>(null);
  const [chunks, setChunks] = useState<Chunk[]>([]);
  const [configs, setConfigs] = useState<Config[]>([]);
  const [results, setResults] = useState<Result[]>([]);
  const [selectedResultId, setSelectedResultId] = useState<string>();
  const [error, setError] = useState<string>();

  const loadDemo = async () => {
    try {
      const demo = await api.demo();
      setCapture(demo.capture);
      setChunks(demo.chunks);
      setConfigs([demo.config]);
      setResults([demo.result]);
      setSelectedResultId(demo.result.id);
    } catch (err) {
      setError(String(err));
    }
  };

  useEffect(() => { void loadDemo(); }, []);

  const createConfig = async (name: string, parameters: UARTParameters) => {
    if (!capture) return;
    const { config, result } = await api.createConfig(capture.id, parameters, name);
    const nextConfigs = [...configs.filter((item) => item.id !== config.id), config];
    const nextResults = [...results.filter((item) => item.id !== result.id), result];
    setConfigs(nextConfigs);
    setResults(nextResults);
    setSelectedResultId(result.id);
  };

  const selectedResult = useMemo(
    () => results.find((result) => result.id === selectedResultId) ?? results[results.length - 1],
    [results, selectedResultId],
  );
  const selectedConfig = configs.find((config) => config.id === selectedResult?.configId);

  if (!capture) return <main className="loading">{error ? <pre>{error}</pre> : '正在载入内置波形…'}</main>;

  return (
    <main>
      <header>
        <div>
          <p className="eyebrow">Hardware Logic Analyzer</p>
          <h1>硬件逻辑分析工作站</h1>
          <p>人工电平采样 · Go 解码 · React 轨道/标记 · 当前协议 UART</p>
        </div>
        <button onClick={loadDemo}>重新生成核对波形</button>
      </header>
      <div className="layout">
        <div className="left-column">
          <ConfigForm channelCount={capture.channelNames.length} initial={selectedConfig} onSubmit={createConfig} />
          <ResultList configs={configs} results={results} selectedId={selectedResult?.id} onSelect={setSelectedResultId} />
        </div>
        <WaveformTrack chunks={chunks} channelNames={capture.channelNames} markers={selectedResult?.markers ?? []} />
      </div>
    </main>
  );
}
