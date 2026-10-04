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

interface DisplaySegment {
  index: number;
  offset: number;
  rawStart: number;
  rawEnd: number;
  displayStart: number;
  displayEnd: number;
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
  // 时钟重置把采样分成多个采集段：各段在显示轴上依次排开，
  // 重置前后相同的时间戳落在不同段，绝不重叠成同一事件。
  const { segments, chunkSegment } = useMemo(() => {
    const segments: DisplaySegment[] = [];
    const chunkSegment = new Map<string, number>();
    let displayEnd = 0;
    for (const chunk of ordered) {
      if (chunk.levels.length === 0) continue;
      const rawEnd = chunk.startTimeNs + Math.max(0, chunk.levels.length - 1) * chunk.intervalNs;
      if (segments.length === 0 || chunk.startTimeNs < segments[segments.length - 1].rawEnd) {
        const offset = segments.length === 0 ? 0 : displayEnd + chunk.intervalNs * 40 - chunk.startTimeNs;
        segments.push({ index: segments.length, offset, rawStart: chunk.startTimeNs, rawEnd: chunk.startTimeNs, displayStart: chunk.startTimeNs + offset, displayEnd: chunk.startTimeNs + offset });
      }
      const segment = segments[segments.length - 1];
      segment.rawEnd = Math.max(segment.rawEnd, rawEnd);
      segment.rawStart = Math.min(segment.rawStart, chunk.startTimeNs);
      segment.displayStart = segment.rawStart + segment.offset;
      segment.displayEnd = segment.rawEnd + segment.offset;
      displayEnd = Math.max(displayEnd, segment.displayEnd);
      chunkSegment.set(chunk.id, segment.index);
    }
    return { segments, chunkSegment };
  }, [ordered]);
  const span = useMemo(() => {
    if (ordered.length === 0) return { start: 0, end: 1, interval: 1 };
    return {
      start: 0,
      end: Math.max(1, ...segments.map((segment) => segment.displayEnd)),
      interval: Math.min(...ordered.map((chunk) => chunk.intervalNs)),
    };
  }, [ordered, segments]);
  const segmentOffset = (index: number | undefined) => segments[index ?? 0]?.offset ?? 0;
  const [viewport, setViewport] = useState<Viewport>(() => ({ startNs: 0, nsPerPixel: Math.max(1, span.end / WIDTH) }));
  const svgRef = useRef<SVGSVGElement>(null);

  useEffect(() => {
    if (!focus) return;
    const { startNs, endNs } = focus.marker;
    const offset = segmentOffset(focus.marker.segmentIndex);
    const center = (startNs + endNs) / 2 + offset;
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
  // 刻度标签显示该位置所属采集段的原生时间。
  const displayToRaw = (display: number) => {
    for (const segment of segments) {
      if (display >= segment.displayStart && display <= segment.displayEnd) {
        return { raw: display - segment.offset, segment };
      }
    }
    let nearest = segments[0];
    for (const segment of segments) {
      if (Math.abs(display - segment.displayEnd) < Math.abs(display - nearest.displayEnd)) nearest = segment;
    }
    return { raw: display - (nearest?.offset ?? 0), segment: nearest };
  };

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
        const offset = segmentOffset(chunkSegment.get(chunk.id));
        chunk.levels.forEach((row, sampleIndex) => {
          const t = chunk.startTimeNs + sampleIndex * chunk.intervalNs + offset;
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

  const visibleMarkers = markers.filter((marker) => {
    const offset = segmentOffset(marker.segmentIndex);
    return marker.endNs + offset >= viewport.startNs && marker.startNs + offset <= xToTime(WIDTH);
  });
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
            <text x={(WIDTH / 10) * index + 4} y={18}>{formatNs(displayToRaw(time).raw)}</text>
          </g>
        ))}
        {segments.filter((segment) => segment.index > 0).map((segment) => (
          <g key={segment.index} className="segment-boundary">
            <line x1={timeToX(segment.displayStart)} y1={24} x2={timeToX(segment.displayStart)} y2={ROW_HEIGHT * channelNames.length + 22} />
            <text x={timeToX(segment.displayStart) + 4} y={ROW_HEIGHT * channelNames.length + 38}>段 {segment.index + 1}（时钟重置）</text>
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
          const offset = segmentOffset(marker.segmentIndex);
          const x = Math.max(0, timeToX(marker.startNs + offset));
          const width = Math.max(2, timeToX(marker.endNs + offset) - x);
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
