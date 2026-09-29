import { useState } from "react";
import { useStation } from "../store";
import type { DecodeConfig } from "../types";

export function ConfigPanel() {
  const {
    sampleConfigs,
    decodeConfigs,
    selectedSample,
    selectedDecode,
    selectSample,
    selectDecode,
    saveDecodeConfig,
    recompute,
    loadDemo,
    busy,
    error,
  } = useStation();
  const [demoText, setDemoText] = useState("Hello UART");
  const [baud, setBaud] = useState(9600);
  const [injectGap, setInjectGap] = useState(false);
  const [injectGlitch, setInjectGlitch] = useState(false);
  const [newVer, setNewVer] = useState("");
  const [newBaud, setNewBaud] = useState(9600);

  const curSample = sampleConfigs.find((c) => c.version === selectedSample) ?? null;
  const channels = curSample?.channels ?? [];
  const curDecode = decodeConfigs.find((c) => c.version === selectedDecode) ?? null;

  const saveVariant = async () => {
    const base: DecodeConfig = curDecode ?? {
      version: newVer || `d-${Date.now()}`,
      protocol: "uart",
      channels: channels.length ? [channels[Math.max(0, channels.indexOf("tx"))]] : ["tx"],
      baudRate: newBaud,
      dataBits: 8,
      parity: "none",
      stopBits: 1,
    };
    const cfg: DecodeConfig = {
      ...base,
      version: newVer || `${base.version}-copy-${Date.now()}`,
      baudRate: newBaud || base.baudRate,
      protocol: "uart",
    };
    await saveDecodeConfig(cfg);
  };

  return (
    <section className="panel">
      <h2>采样与解码配置（版本分离）</h2>

      <div className="row">
        <strong>自带波形：</strong>
        <input value={demoText} onChange={(e) => setDemoText(e.target.value)} size={12} />
        <label>
          波特率 <input type="number" value={baud} onChange={(e) => setBaud(+e.target.value)} />
        </label>
        <label>
          <input type="checkbox" checked={injectGap} onChange={(e) => setInjectGap(e.target.checked)} /> 注入采样间隙
        </label>
        <label>
          <input
            type="checkbox"
            checked={injectGlitch}
            onChange={(e) => setInjectGlitch(e.target.checked)}
          />{" "}
          注入时钟异常
        </label>
        <button
          disabled={busy}
          onClick={() =>
            loadDemo({
              text: demoText,
              baudRate: baud,
              channels: ["clk", "tx", "rx"],
              channel: "tx",
              gapAfter: injectGap ? 700 : 0,
              gapSkip: injectGap ? 600 : 0,
              glitchFrame: injectGlitch ? 1 : -1,
              glitchBit: injectGlitch ? 2 : 0,
              glitchBySamples: injectGlitch ? 2.5 : 0,
            })
          }
        >
          生成并载入波形
        </button>
      </div>

      <div className="row">
        <strong>采样配置版本：</strong>
        <select value={selectedSample ?? ""} onChange={(e) => selectSample(e.target.value)}>
          <option value="">（未选择）</option>
          {sampleConfigs.map((c) => (
            <option key={c.version} value={c.version}>
              {c.version} · {c.channels.join("/")} · {c.periodNs}ns
            </option>
          ))}
        </select>
      </div>

      <div className="row">
        <strong>解码配置版本：</strong>
        <select value={selectedDecode ?? ""} onChange={(e) => selectDecode(e.target.value)}>
          <option value="">（未选择）</option>
          {decodeConfigs.map((c) => (
            <option key={c.version} value={c.version}>
              {c.version} · {c.protocol} · {c.baudRate}bps
            </option>
          ))}
        </select>
        <button disabled={busy || !selectedSample || !selectedDecode} onClick={recompute}>
          用当前配置重算
        </button>
      </div>

      <div className="row">
        <strong>另存为新版本：</strong>
        <input
          placeholder="新版本号，如 d-19200"
          value={newVer}
          onChange={(e) => setNewVer(e.target.value)}
        />
        <label>
          波特率 <input type="number" value={newBaud} onChange={(e) => setNewBaud(+e.target.value)} />
        </label>
        <button disabled={busy || channels.length === 0} onClick={saveVariant}>
          保存解码配置版本
        </button>
        <span className="hint">修改配置只产生新版本；旧结果保留，可在“标记/对比”页比较。</span>
      </div>

      {error && <div className="error">{error}</div>}
    </section>
  );
}
