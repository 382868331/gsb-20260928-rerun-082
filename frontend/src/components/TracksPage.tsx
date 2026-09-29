import { useState } from "react";
import { useStation } from "../store";
import { ConfigPanel } from "./ConfigPanel";
import { Waveform } from "./Waveform";
import { formatNs } from "../time";

export function TracksPage() {
  const { sampleConfigs, selectedSample, blocks, result } = useStation();
  const cfg = sampleConfigs.find((c) => c.version === selectedSample) ?? null;
  const [channel, setChannel] = useState("tx");

  const activeChannel = cfg?.channels.includes(channel) ? channel : cfg?.channels[0] ?? "";
  const bytes = result ? result.bytes.map((b) => b.toString(16).padStart(2, "0")).join(" ") : "";

  return (
    <div>
      <ConfigPanel />
      <section className="panel">
        <h2>轨道</h2>
        <div className="row">
          <strong>通道：</strong>
          {(cfg?.channels ?? []).map((c) => (
            <label key={c}>
              <input
                type="radio"
                name="ch"
                checked={activeChannel === c}
                onChange={() => setChannel(c)}
              />{" "}
              {c}
            </label>
          ))}
          <span className="hint">块数：{blocks.length}</span>
          {result && (
            <span className="hint">
              结果版本 {result.version.slice(0, 14)}… · 字节 {result.bytes.length} · 错误 {result.errorCount}
            </span>
          )}
        </div>
        {cfg ? (
          <Waveform
            cfg={cfg}
            blocks={blocks}
            result={result}
            selectedChannel={activeChannel}
          />
        ) : (
          <p className="hint">先在上方“生成并载入波形”，再选择解码配置并重算。</p>
        )}
        {result && (
          <div className="decodeout">
            <div>
              <strong>解码字节：</strong>
              <code>{bytes || "（无）"}</code>
            </div>
            <div>
              <strong>文本：</strong> <code>{result.bytes.map((b) => (b >= 32 && b < 127 ? String.fromCharCode(b) : "·")).join("")}</code>
            </div>
            <div className="hint">
              首帧 {result.items.find((i) => i.kind === "data") ? formatNs(result.items.find((i) => i.kind === "data")!.startNs) : "-"}
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
