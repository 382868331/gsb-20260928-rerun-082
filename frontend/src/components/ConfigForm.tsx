import { useState } from 'react';
import type { Config, SPIParameters, UARTParameters } from '../types';
import type { NewConfigBody } from '../lib/api';

interface Props {
  channelCount: number;
  onSubmit: (body: NewConfigBody) => Promise<void>;
  initial?: Config;
}

function ChannelSelect({ channelCount, value, onChange, allowNone }: { channelCount: number; value: number; onChange: (v: number) => void; allowNone?: boolean }) {
  return (
    <select value={value} onChange={(e) => onChange(Number(e.target.value))}>
      {allowNone && <option value={-1}>无</option>}
      {Array.from({ length: channelCount }, (_, i) => <option key={i} value={i}>CH{i}</option>)}
    </select>
  );
}

export function ConfigForm({ channelCount, onSubmit, initial }: Props) {
  const [protocol, setProtocol] = useState<'uart' | 'spi'>(initial?.protocol ?? 'uart');
  const [name, setName] = useState(initial?.name ?? '');
  const [busy, setBusy] = useState(false);

  const [channel, setChannel] = useState(initial?.parameters.channel ?? 1);
  const [baud, setBaud] = useState(initial?.parameters.baud ?? 9600);
  const [dataBits, setDataBits] = useState(initial?.parameters.dataBits ?? 8);
  const [parity, setParity] = useState<UARTParameters['parity']>(initial?.parameters.parity ?? 'none');
  const [stopBits, setStopBits] = useState<1 | 2>(initial?.parameters.stopBits ?? 1);
  const [idleLevel, setIdleLevel] = useState<0 | 1>(initial?.parameters.idleLevel ?? 1);
  const [clockTolerance, setClockTolerance] = useState(initial?.parameters.clockTolerance ?? 0.03);

  const spi = initial?.spiParameters;
  const [clockChannel, setClockChannel] = useState(spi?.clockChannel ?? 0);
  const [dataChannel, setDataChannel] = useState(spi?.dataChannel ?? 1);
  const [csChannel, setCsChannel] = useState(spi?.csChannel ?? 2);
  const [csActive, setCsActive] = useState<0 | 1>(spi?.csActive ?? 0);
  const [bitOrder, setBitOrder] = useState<SPIParameters['bitOrder']>(spi?.bitOrder ?? 'msb');
  const [clockEdge, setClockEdge] = useState<SPIParameters['clockEdge']>(spi?.clockEdge ?? 'rising');
  const [bitsPerWord, setBitsPerWord] = useState(spi?.bitsPerWord ?? 8);

  const submit = async () => {
    setBusy(true);
    try {
      const finalName = name || (protocol === 'uart' ? 'UART 配置' : 'SPI 配置') + ' ' + new Date().toLocaleTimeString();
      if (protocol === 'uart') {
        await onSubmit({ protocol, name: finalName, parameters: { channel, baud, dataBits, parity, stopBits, idleLevel, clockTolerance } });
      } else {
        await onSubmit({ protocol, name: finalName, spiParameters: { clockChannel, dataChannel, csChannel, csActive, bitOrder, clockEdge, bitsPerWord } });
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="panel">
      <h2>采样与解码配置</h2>
      <label>名称<input value={name} placeholder="留空自动命名" onChange={(e) => setName(e.target.value)} /></label>
      <label>协议
        <select value={protocol} onChange={(e) => setProtocol(e.target.value as 'uart' | 'spi')}>
          <option value="uart">UART（异步串行）</option>
          <option value="spi">SPI（时钟同步串行）</option>
        </select>
      </label>
      {protocol === 'uart' ? (
        <div className="form-grid">
          <label>通道<ChannelSelect channelCount={channelCount} value={channel} onChange={setChannel} /></label>
          <label>波特率<input type="number" value={baud} onChange={(e) => setBaud(Number(e.target.value))} /></label>
          <label>数据位<select value={dataBits} onChange={(e) => setDataBits(Number(e.target.value))}>{[5, 6, 7, 8, 9].map((n) => <option key={n}>{n}</option>)}</select></label>
          <label>校验<select value={parity} onChange={(e) => setParity(e.target.value as UARTParameters['parity'])}><option value="none">none</option><option value="even">even</option><option value="odd">odd</option><option value="mark">mark</option><option value="space">space</option></select></label>
          <label>停止位<select value={stopBits} onChange={(e) => setStopBits(Number(e.target.value) as 1 | 2)}><option value={1}>1</option><option value={2}>2</option></select></label>
          <label>空闲电平<select value={idleLevel} onChange={(e) => setIdleLevel(Number(e.target.value) as 0 | 1)}><option value={1}>高</option><option value={0}>低</option></select></label>
          <label>时钟容差<input type="number" step={0.005} value={clockTolerance} onChange={(e) => setClockTolerance(Number(e.target.value))} /></label>
        </div>
      ) : (
        <div className="form-grid">
          <label>时钟通道<ChannelSelect channelCount={channelCount} value={clockChannel} onChange={setClockChannel} /></label>
          <label>数据通道<ChannelSelect channelCount={channelCount} value={dataChannel} onChange={setDataChannel} /></label>
          <label>片选通道<ChannelSelect channelCount={channelCount} value={csChannel} onChange={setCsChannel} allowNone /></label>
          <label>片选有效电平<select value={csActive} onChange={(e) => setCsActive(Number(e.target.value) as 0 | 1)}><option value={0}>低有效</option><option value={1}>高有效</option></select></label>
          <label>位序<select value={bitOrder} onChange={(e) => setBitOrder(e.target.value as SPIParameters['bitOrder'])}><option value="msb">MSB 在前</option><option value="lsb">LSB 在前</option></select></label>
          <label>采样时钟边沿<select value={clockEdge} onChange={(e) => setClockEdge(e.target.value as SPIParameters['clockEdge'])}><option value="rising">上升沿</option><option value="falling">下降沿</option></select></label>
          <label>每字位数<select value={bitsPerWord} onChange={(e) => setBitsPerWord(Number(e.target.value))}>{[4, 8, 12, 16, 24, 32].map((n) => <option key={n}>{n}</option>)}</select></label>
        </div>
      )}
      <button disabled={busy} onClick={submit}>{busy ? '重算中…' : '另存新版本并重算'}</button>
    </section>
  );
}
