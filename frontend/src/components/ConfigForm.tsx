import { useState } from 'react';
import type { Config, UARTParameters } from '../types';

interface Props {
  channelCount: number;
  onSubmit: (name: string, parameters: UARTParameters) => Promise<void>;
  initial?: Config;
}

export function ConfigForm({ channelCount, onSubmit, initial }: Props) {
  const [name, setName] = useState(initial?.name ?? `UART 配置 ${new Date().toLocaleTimeString()}`);
  const [channel, setChannel] = useState(initial?.parameters.channel ?? 1);
  const [baud, setBaud] = useState(initial?.parameters.baud ?? 9600);
  const [dataBits, setDataBits] = useState(initial?.parameters.dataBits ?? 8);
  const [parity, setParity] = useState<UARTParameters['parity']>(initial?.parameters.parity ?? 'none');
  const [stopBits, setStopBits] = useState<1 | 2>(initial?.parameters.stopBits ?? 1);
  const [idleLevel, setIdleLevel] = useState<0 | 1>(initial?.parameters.idleLevel ?? 1);
  const [clockTolerance, setClockTolerance] = useState(initial?.parameters.clockTolerance ?? 0.03);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    try {
      await onSubmit(name, { channel, baud, dataBits, parity, stopBits, idleLevel, clockTolerance });
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="panel">
      <h2>采样与解码配置</h2>
      <label>名称<input value={name} onChange={(e) => setName(e.target.value)} /></label>
      <label>协议<input value="UART（当前唯一明确支持）" disabled /></label>
      <div className="form-grid">
        <label>通道<select value={channel} onChange={(e) => setChannel(Number(e.target.value))}>{Array.from({ length: channelCount }, (_, i) => <option key={i} value={i}>CH{i}</option>)}</select></label>
        <label>波特率<input type="number" value={baud} onChange={(e) => setBaud(Number(e.target.value))} /></label>
        <label>数据位<select value={dataBits} onChange={(e) => setDataBits(Number(e.target.value))}>{[5, 6, 7, 8, 9].map((n) => <option key={n}>{n}</option>)}</select></label>
        <label>校验<select value={parity} onChange={(e) => setParity(e.target.value as UARTParameters['parity'])}><option value="none">none</option><option value="even">even</option><option value="odd">odd</option><option value="mark">mark</option><option value="space">space</option></select></label>
        <label>停止位<select value={stopBits} onChange={(e) => setStopBits(Number(e.target.value) as 1 | 2)}><option value={1}>1</option><option value={2}>2</option></select></label>
        <label>空闲电平<select value={idleLevel} onChange={(e) => setIdleLevel(Number(e.target.value) as 0 | 1)}><option value={1}>高</option><option value={0}>低</option></select></label>
        <label>时钟容差<input type="number" step={0.005} value={clockTolerance} onChange={(e) => setClockTolerance(Number(e.target.value))} /></label>
      </div>
      <button disabled={busy} onClick={submit}>{busy ? '重算中…' : '另存新版本并重算'}</button>
    </section>
  );
}
