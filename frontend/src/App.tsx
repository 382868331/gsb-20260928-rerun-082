import { useEffect, useMemo, useState } from 'react';
import { api } from './lib/api';
import { ConfigForm } from './components/ConfigForm';
import { ResultList } from './components/ResultList';
import { WaveformTrack } from './components/WaveformTrack';
import type { DemoCapture, Marker, SPIParameters, UARTParameters } from './types';

interface FocusTarget {
  marker: Marker;
  seq: number;
}

export default function App() {
  const [demos, setDemos] = useState<DemoCapture[]>([]);
  const [activeCaptureId, setActiveCaptureId] = useState<string>();
  const [selectedResultId, setSelectedResultId] = useState<string>();
  const [focus, setFocus] = useState<FocusTarget>();
  const [error, setError] = useState<string>();

  const loadDemo = async () => {
    try {
      const demo = await api.demo();
      setDemos(demo.captures);
      setActiveCaptureId(demo.captures[0]?.capture.id);
      setSelectedResultId(demo.captures[0]?.results[0]?.id);
    } catch (err) {
      setError(String(err));
    }
  };

  useEffect(() => { void loadDemo(); }, []);

  const active = useMemo(
    () => demos.find((demo) => demo.capture.id === activeCaptureId) ?? demos[0],
    [demos, activeCaptureId],
  );

  const createConfig = async (name: string, protocol: string, parameters: UARTParameters, spiParameters?: SPIParameters) => {
    if (!active) return;
    const { config, result } = await api.createConfig(active.capture.id, protocol, name, parameters, spiParameters);
    setDemos((current) => current.map((demo) => {
      if (demo.capture.id !== active.capture.id) return demo;
      return {
        ...demo,
        configs: [...demo.configs.filter((item) => item.id !== config.id), config],
        results: [...demo.results.filter((item) => item.id !== result.id), result],
      };
    }));
    setSelectedResultId(result.id);
  };

  const locate = (marker: Marker) => {
    setFocus((current) => ({ marker, seq: (current?.seq ?? 0) + 1 }));
  };

  const selectedResult = useMemo(
    () => active?.results.find((result) => result.id === selectedResultId) ?? active?.results[active.results.length - 1],
    [active, selectedResultId],
  );
  const selectedConfig = active?.configs.find((config) => config.id === selectedResult?.configId);

  if (!active) return <main className="loading">{error ? <pre>{error}</pre> : '正在载入内置波形…'}</main>;

  return (
    <main>
      <header>
        <div>
          <p className="eyebrow">Hardware Logic Analyzer</p>
          <h1>硬件逻辑分析工作站</h1>
          <p>人工电平采样 · Go 解码 · React 轨道/标记 · 协议 UART / SPI</p>
        </div>
        <button onClick={loadDemo}>重新生成核对波形</button>
      </header>
      <nav className="capture-tabs">
        {demos.map((demo) => (
          <button
            key={demo.capture.id}
            className={demo.capture.id === active.capture.id ? 'selected' : ''}
            onClick={() => {
              setActiveCaptureId(demo.capture.id);
              setSelectedResultId(demo.results[0]?.id);
            }}
          >
            {demo.capture.name}
          </button>
        ))}
      </nav>
      <div className="layout">
        <div className="left-column">
          <ConfigForm channelCount={active.capture.channelNames.length} initial={selectedConfig} onSubmit={createConfig} />
          <ResultList
            configs={active.configs}
            results={active.results}
            selectedId={selectedResult?.id}
            onSelect={setSelectedResultId}
            onLocate={locate}
          />
        </div>
        <WaveformTrack
          chunks={active.chunks}
          channelNames={active.capture.channelNames}
          markers={selectedResult?.markers ?? []}
          focus={focus}
        />
      </div>
    </main>
  );
}
