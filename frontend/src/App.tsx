import { useEffect, useMemo, useState } from 'react';
import { api } from './lib/api';
import { ConfigForm } from './components/ConfigForm';
import { ResultList } from './components/ResultList';
import { WaveformTrack } from './components/WaveformTrack';
import { DiffPanel } from './components/DiffPanel';
import { AlignmentPanel } from './components/AlignmentPanel';
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
  const [captureEpoch, setCaptureEpoch] = useState(0);

  const loadDemo = async () => {
    try {
      const demo = await api.demo();
      setDemos(demo.captures);
      setActiveCaptureId(demo.captures[0]?.capture.id);
      setSelectedResultId(demo.captures[0]?.results[0]?.id);
      setCaptureEpoch((n) => n + 1);
    } catch (err) {
      setError(String(err));
    }
  };

  useEffect(() => { void loadDemo(); }, []);

  const active = useMemo(
    () => demos.find((demo) => demo.capture.id === activeCaptureId),
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

  // 跨采样定位：切换到另一份采样并聚焦映射区间；不重置该采样视口状态之外的内容，
  // 各采样的配置与结果版本始终按采样身份分开，绝不混用。
  const locateCross = (captureId: string, resultId: string | undefined, marker: Marker) => {
    setActiveCaptureId(captureId);
    if (resultId) setSelectedResultId(resultId);
    setFocus((current) => ({ marker, seq: (current?.seq ?? 0) + 1 }));
  };

  const selectedResult = useMemo(
    () => active?.results.find((result) => result.id === selectedResultId),
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
        <button onClick={loadDemo}>重开内置波形</button>
      </header>
      <nav className="capture-tabs">
        {demos.map((demo) => (
          <button
            key={demo.capture.id}
            className={demo.capture.id === active.capture.id ? 'selected' : ''}
            onClick={() => {
              setActiveCaptureId(demo.capture.id);
              setSelectedResultId(demo.results[0]?.id);
              setFocus(undefined);
              setCaptureEpoch((n) => n + 1);
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
          <DiffPanel
            key={`diff-${active.capture.id}`}
            configs={active.configs}
            results={active.results}
            selectedResultId={selectedResult?.id}
            onLocate={locate}
          />
        </div>
        <WaveformTrack
          chunks={active.chunks}
          channelNames={active.capture.channelNames}
          markers={selectedResult?.markers ?? []}
          focus={focus}
          resetEpoch={captureEpoch}
        />
      </div>
      <AlignmentPanel demos={demos} onLocateCross={locateCross} />
    </main>
  );
}
