import { useEffect, useMemo, useState } from 'react';
import { api } from './lib/api';
import { ConfigForm } from './components/ConfigForm';
import { DiffPanel } from './components/DiffPanel';
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

  // 重新打开服务器上已保存的采样、配置与结果版本；
  // 仅在没有任何采样记录时才生成内置示例，绝不覆盖已有对照。
  const loadCaptures = async () => {
    try {
      let captures = await api.captures();
      if (captures.length === 0) {
        const demo = await api.demo();
        setDemos(demo.captures);
        setActiveCaptureId((current) => current ?? demo.captures[0]?.capture.id);
        setSelectedResultId((current) => current ?? demo.captures[0]?.results[0]?.id);
        return;
      }
      const restored: DemoCapture[] = await Promise.all(
        captures.map(async (capture) => {
          const [detail, configs, results] = await Promise.all([
            api.capture(capture.id),
            api.configs(capture.id),
            api.results(capture.id),
          ]);
          return { capture: detail.capture, chunks: detail.chunks, configs, results };
        }),
      );
      setDemos(restored);
      setActiveCaptureId((current) =>
        restored.some((demo) => demo.capture.id === current) ? current : restored[0]?.capture.id,
      );
      setSelectedResultId((current) =>
        restored.some((demo) => demo.results.some((result) => result.id === current))
          ? current
          : undefined,
      );
    } catch (err) {
      setError(String(err));
    }
  };

  useEffect(() => { void loadCaptures(); }, []);

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
        <button onClick={loadCaptures}>重新打开已保存对照</button>
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
          <ConfigForm
            key={`${active.capture.id}:${selectedConfig?.id ?? 'new'}`}
            channelCount={active.capture.channelNames.length}
            initial={selectedConfig}
            onSubmit={createConfig}
          />
          <ResultList
            configs={active.configs}
            results={active.results}
            selectedId={selectedResult?.id}
            onSelect={setSelectedResultId}
            onLocate={locate}
          />
          <DiffPanel results={active.results} onLocate={locate} />
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
