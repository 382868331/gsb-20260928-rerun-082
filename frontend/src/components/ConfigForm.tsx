import { useState } from 'react';
import type { Config, SPIParameters, UARTParameters } from '../types';

interface Props {
  channelCount: number;
  onSubmit: (name: string, protocol: string, parameters: UARTParameters, spiParameters?: SPIParameters) => Promise<void>;
  initial?: Config;
}

export function ConfigForm({ channelCount, onSubmit, initial }: Props) {
  const [name, setName] = useState(initial?.name ?? `解码配置 ${new Date().toLocaleTimeString()}`);
  const [protocol, setProtocol] = useState(initial?.protocol ?? 'uart');
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
  const [clockEdge, setClockEdge] = useState<'rising' | 'falling'>(spi?.clockEdge ?? 'rising');
  const [bitOrder, setBitOrder] = useState<'msb' | 'lsb'>(spi?.bitOrder ?? 'msb');
  const [bitsPerWord, setBitsPerWord] = useState(spi?.bitsPerWord ?? 8);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    try {
      const uart: UARTParameters = { channel, baud, dataBits, parity, stopBits, idleLevel, clockTolerance };
      const spiParameters: SPIParameters | undefined = protocol === 'spi'
        ? { clockChannel, dataChannel, csChannel, csActive, clockEdge, bitOrder, bitsPerWord }
        : undefined;
      await onSubmit(name, protocol, uart, spiParameters);
    } finally {
      setBusy(false);
    }
  };

  const channelOptions = (allowNone: boolean) => (
    <>
      {allowNone && <option value={-1}>不使用</option>}
      {Array.from({ length: channelCount }, (_, i) => <option key={i} value={i}>CH{i}</option>)}
    </>
  );

  return (
    <section className="panel">
      <h2>采样与解码配置</h2>
      <label>名称<input value={name} onChange={(e) => setName(e.target.value)} /></label>
      <label>协议
        <select value={protocol} onChange={(e) => setProtocol(e.target.value)}>
          <option value="uart">UART（异步）</option>
          <option value="spi">SPI（时钟同步）</option>
        </select>
      </label>
      {protocol === 'uart' ? (
        <div className="form-grid">
          <label>通道<select value={channel} onChange={(e) => setChannel(Number(e.target.value))}>{channelOptions(false)}</select></label>
          <label>波特率<input type="number" value={baud} onChange={(e) => setBaud(Number(e.target.value))} /></label>
          <label>数据位<select value={dataBits} onChange={(e) => setDataBits(Number(e.target.value))}>{[5, 6, 7, 8, 9].map((n) => <option key={n}>{n}</option>)}</select></label>
          <label>校验<select value={parity} onChange={(e) => setParity(e.target.value as UARTParameters['parity'])}><option value="none">none</option><option value="even">even</option><option value="odd">odd</option><option value="mark">mark</option><option value="space">space</option></select></label>
          <label>停止位<select value={stopBits} onChange={(e) => setStopBits(Number(e.target.value) as 1 | 2)}><option value={1}>1</option><option value={2}>2</option></select></label>
          <label>空闲电平<select value={idleLevel} onChange={(e) => setIdleLevel(Number(e.target.value) as 0 | 1)}><option value={1}>高</option><option value={0}>低</option></select></label>
          <label>时钟容差<input type="number" step={0.005} value={clockTolerance} onChange={(e) => setClockTolerance(Number(e.target.value))} /></label>
        </div>
      ) : (
        <div className="form-grid">
          <label>时钟通道<select value={clockChannel} onChange={(e) => setClockChannel(Number(e.target.value))}>{channelOptions(false)}</select></label>
          <label>数据通道<select value={dataChannel} onChange={(e) => setDataChannel(Number(e.target.value))}>{channelOptions(false)}</select></label>
          <label>片选通道<select value={csChannel} onChange={(e) => setCsChannel(Number(e.target.value))}>{channelOptions(true)}</select></label>
          <label>片选有效电平<select value={csActive} onChange={(e) => setCsActive(Number(e.target.value) as 0 | 1)}><option value={0}>低有效</option><option value={1}>高有效</option></select></label>
          <label>采样边沿<select value={clockEdge} onChange={(e) => setClockEdge(e.target.value as 'rising' | 'falling')}><option value="rising">上升沿</option><option value="falling">下降沿</option></select></label>
          <label>位序<select value={bitOrder} onChange={(e) => setBitOrder(e.target.value as 'msb' | 'lsb')}><option value="msb">MSB 先传</option><option value="lsb">LSB 先传</option></select></label>
          <label>每字位数<input type="number" value={bitsPerWord} onChange={(e) => setBitsPerWord(Number(e.target.value))} /></label>
        </div>
      )}
      <button disabled={busy} onClick={submit}>{busy ? '重算中…' : '另存新版本并重算'}</button>
    </section>
  );
}
