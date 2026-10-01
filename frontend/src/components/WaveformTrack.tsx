import { useEffect, useMemo, useRef, useState } from 'react';
import type { Chunk, Marker } from '../types';

interface Props {
  chunks: Chunk[];
  channelNames: string[];
  markers: Marker[];
  focus?: { marker: Marker; seq: number };
  resetEpoch?: number;
}

interface Viewport {
  startNs: number;
  nsPerPixel: number;
}

const WIDTH = 980;
const ROW_HEIGHT = 108;

function formatNs(ns: number): string {
  if (ns >= 1_000_000) return `${(ns / 1_000_000).toFixed(3)} ms`;
  if (ns >= 1_000) return `${(ns / 1_000).toFixed(2)} µs`;
  return `${ns.toFixed(0)} ns`;
}

export function WaveformTrack({ chunks, channelNames, markers, focus, resetEpoch }: Props) {
  const ordered = useMemo(() => [...chunks].sort((a, b) => a.index - b.index), [chunks]);
  const span = useMemo(() => {
    if (ordered.length === 0) return { start: 0, end: 1, interval: 1 };
    const last = ordered[ordered.length - 1];
    return {
      start: Math.min(...ordered.map((chunk) => chunk.startTimeNs)),
      end: last.startTimeNs + Math.max(0, last.levels.length - 1) * last.intervalNs,
      interval: Math.min(...ordered.map((chunk) => chunk.intervalNs)),
    };
  }, [ordered]);
  const [viewport, setViewport] = useState<Viewport>(() => ({ startNs: 0, nsPerPixel: Math.max(1, span.end / WIDTH) }));
  const svgRef = useRef<SVGSVGElement>(null);

  useEffect(() => {
    if (!focus) return;
    const { startNs, endNs } = focus.marker;
    const center = (startNs + endNs) / 2;
    const width = Math.max(endNs - startNs, span.interval) * 6;
    const nsPerPixel = Math.min(Math.max(span.interval / 8, width / WIDTH), span.end / 80);
    setViewport({ startNs: center - (nsPerPixel * WIDTH) / 2, nsPerPixel });
    // 仅在用户点击定位时触发
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focus?.seq]);

  // 切换采样后回到该采样自身的全览视口；缩放/平移状态绝不跨采样携带，也不触碰已保存结果。
  useEffect(() => {
    setViewport({ startNs: 0, nsPerPixel: Math.max(1, span.end / WIDTH) });
    // span 由当前采样决定；仅在显式切换/重开时重置，避免缩放后 span 引用变化抖动。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetEpoch]);

  const timeToX = (time: number) => (time - viewport.startNs) / viewport.nsPerPixel;
  const xToTime = (x: number) => viewport.startNs + x * viewport.nsPerPixel;

  const zoomAt = (clientX: number, factor: number) => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return;
    const x = clientX - rect.left;
    const anchor = xToTime(x);
    setViewport((current) => {
      const nextScale = Math.min(Math.max(span.interval / 8, current.nsPerPixel * factor), span.end / 80);
      return { startNs: anchor - x * nextScale, nsPerPixel: nextScale };
    });
  };

  const pan = (deltaPixels: number) => {
    setViewport((current) => ({ ...current, startNs: current.startNs + deltaPixels * current.nsPerPixel }));
  };

  const paths = useMemo(() => {
	return channelNames.map((_, channel) => {
      const points: Array<{ x: number; y: number }> = [];
      ordered.forEach((chunk) => {
        chunk.levels.forEach((row, sampleIndex) => {
          const t = chunk.startTimeNs + sampleIndex * chunk.intervalNs;
          const x = timeToX(t);
          const y = channel * ROW_HEIGHT + 72 - (row[channel] === 1 ? 38 : 10);
          const last = points[points.length - 1];
          if (!last) {
            points.push({ x, y });
          } else if (last.y !== y) {
            points.push({ x, y: last.y }, { x, y });
          } else {
            points.push({ x, y });
          }
        });
      });
      return points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x} ${point.y}`).join(' ');
    });
  }, [channelNames, ordered, viewport]);

  const visibleMarkers = markers.filter((marker) => marker.endNs >= viewport.startNs && marker.startNs <= xToTime(WIDTH));
  const ticks = Array.from({ length: 11 }, (_, i) => xToTime((WIDTH / 10) * i));

  return (
    <section className="panel waveform">
      <div className="section-title">
        <h2>波形轨道与标记</h2>
        <div>
          <button onClick={() => zoomAt(WIDTH / 2, 0.7)}>放大</button>
          <button onClick={() => zoomAt(WIDTH / 2, 1.4)}>缩小</button>
          <button onClick={() => pan(-80)}>前移</button>
          <button onClick={() => pan(80)}>后移</button>
          <button onClick={() => setViewport({ startNs: 0, nsPerPixel: Math.max(1, span.end / WIDTH) })}>全览</button>
        </div>
      </div>
      <svg
        ref={svgRef}
        width={WIDTH}
        height={ROW_HEIGHT * channelNames.length + 44}
        onWheel={(event) => {
          event.preventDefault();
          zoomAt(event.clientX, event.deltaY < 0 ? 0.8 : 1.25);
        }}
      >
        {ticks.map((time, index) => (
          <g key={index}>
            <line x1={(WIDTH / 10) * index} y1={28} x2={(WIDTH / 10) * index} y2={ROW_HEIGHT * channelNames.length + 22} className="tick" />
            <text x={(WIDTH / 10) * index + 4} y={18}>{formatNs(time)}</text>
          </g>
        ))}
        {channelNames.map((name, channel) => (
          <g key={name} transform={`translate(0 ${channel * ROW_HEIGHT + 10})`}>
            <rect x={0} y={22} width={WIDTH} height={78} className="track-bg" />
            <text x={10} y={42}>{name}</text>
            <line x1={0} y1={60} x2={WIDTH} y2={60} className="idle-line" />
            <path d={paths[channel]} className="signal" />
          </g>
        ))}
        {visibleMarkers.map((marker) => {
          const x = Math.max(0, timeToX(marker.startNs));
          const width = Math.max(2, timeToX(marker.endNs) - x);
          const y = marker.channel * ROW_HEIGHT + 30;
          return (
            <g key={marker.id} className={marker.kind === 'error' ? 'marker-error' : 'marker-byte'}>
              <rect x={x} y={y} width={width} height={70} />
              <text x={x + 4} y={y + 16}>{marker.label}</text>
            </g>
          );
        })}
      </svg>
      <p>滚轮缩放以鼠标位置为时间锚点，平移和缩放不会改变解码结果。</p>
    </section>
  );
}
