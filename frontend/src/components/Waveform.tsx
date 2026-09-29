import { useMemo, useRef, useState, useCallback, useEffect } from "react";
import type { DecodeResult, Item, SampleConfig } from "../types";
import type { SampleBlock } from "../types";
import { SampleIndex } from "../samples";
import { nsToPx, panPx, pxToNs, zoomAt, formatNs, type Viewport } from "../time";

interface Props {
  cfg: SampleConfig | null;
  blocks: SampleBlock[];
  result: DecodeResult | null;
  selectedChannel: string;
  onCursor?: (ns: number | null) => void;
}

const ROW_H = 64;
const LABEL_W = 64;
const TOP_PAD = 24;

export function Waveform({ cfg, blocks, result, selectedChannel, onCursor }: Props) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(900);
  const [vp, setVp] = useState<Viewport>({ startNs: 0, durationNs: 10_000_000 });
  const [cursor, setCursor] = useState<number | null>(null);
  const drag = useRef<{ x: number; startNs: number } | null>(null);

  const idx = useMemo(() => new SampleIndex(blocks, cfg), [blocks, cfg]);
  const maxNs = Math.max(vp.durationNs, (idx.lastIndex + 2) * (cfg?.periodNs ?? 1000));
  const minDur = (cfg?.periodNs ?? 1) * 4;
  const maxDur = Math.max(maxNs, minDur * 10);

  // Fit whole capture whenever the sample set changes.
  useEffect(() => {
    setVp({ startNs: 0, durationNs: maxDur });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cfg?.version, blocks]);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const plotW = Math.max(50, width - LABEL_W);
  const chanIdx = cfg ? Math.max(0, cfg.channels.indexOf(selectedChannel)) : 0;

  const onWheel = useCallback(
    (e: React.WheelEvent) => {
      const rect = e.currentTarget.getBoundingClientRect();
      const x = e.clientX - rect.left - LABEL_W;
      const anchorNs = pxToNs(x, plotW, vp);
      const fraction = x / plotW;
      const factor = e.deltaY < 0 ? 1.25 : 1 / 1.25;
      // The pointer time is the anchor and stays under the pointer.
      setVp(zoomAt(vp, anchorNs, factor, minDur, maxDur, fraction));
    },
    [vp, plotW, minDur, maxDur],
  );

  const onDown = (e: React.MouseEvent) => {
    drag.current = { x: e.clientX, startNs: vp.startNs };
  };
  const onMove = (e: React.MouseEvent) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left - LABEL_W;
    const t = pxToNs(x, plotW, vp);
    setCursor(t);
    onCursor?.(t);
    if (drag.current) {
      const dx = e.clientX - drag.current.x;
      setVp(panPx({ ...vp, startNs: drag.current.startNs }, dx, plotW));
    }
  };
  const onUp = () => {
    drag.current = null;
  };

  // Build the level polyline for the selected channel within the view.
  const line = useMemo(() => {
    if (!cfg) return "";
    const p = cfg.periodNs;
    const i0 = Math.max(0, Math.floor(vp.startNs / p) - 2);
    const i1 = Math.min(idx.lastIndex + 1, Math.ceil((vp.startNs + vp.durationNs) / p) + 2);
    const yFor = (lvl: number | null) => (lvl === 1 ? TOP_PAD + 12 : lvl === 0 ? TOP_PAD + ROW_H - 24 : TOP_PAD + ROW_H / 2);
    const pts: string[] = [];
    let prevLevel: number | null = null;
    const maxPts = 4000;
    const step = Math.max(1, Math.floor((i1 - i0) / maxPts));
    for (let i = i0; i <= i1; i += step) {
      const lvl = idx.level(i, chanIdx);
      const x = nsToPx(i * p, plotW, vp);
      if (prevLevel !== null && lvl !== prevLevel) {
        pts.push(`${x},${yFor(prevLevel)}`);
      }
      pts.push(`${x},${yFor(lvl)}`);
      prevLevel = lvl;
    }
    return pts.join(" ");
  }, [cfg, idx, vp, plotW, chanIdx]);

  // Shade capture gaps: covered intervals come from blocks.
  const gaps = useMemo(() => {
    if (!cfg) return [];
    const out: { x: number; w: number }[] = [];
    const covered = idx.covered();
    for (let k = 0; k < covered.length - 1; k++) {
      const gapStartNs = covered[k].end * cfg.periodNs;
      const gapEndNs = covered[k + 1].start * cfg.periodNs;
      const x1 = nsToPx(gapStartNs, plotW, vp);
      const x2 = nsToPx(gapEndNs, plotW, vp);
      if (x2 > 0 && x1 < plotW) out.push({ x: x1, w: Math.max(2, x2 - x1) });
    }
    return out;
  }, [cfg, idx, vp, plotW]);

  const items = result?.items ?? [];
  const visibleItems = items.filter(
    (it) => it.endNs >= vp.startNs && it.startNs <= vp.startNs + vp.durationNs && it.channel === selectedChannel,
  );

  return (
    <div className="waveform" ref={wrapRef}>
      <svg
        width={width}
        height={ROW_H + TOP_PAD + 30}
        onWheel={onWheel}
        onMouseDown={onDown}
        onMouseMove={onMove}
        onMouseUp={onUp}
        onMouseLeave={() => {
          setCursor(null);
          onCursor?.(null);
          drag.current = null;
        }}
        style={{ cursor: drag.current ? "grabbing" : "crosshair", display: "block" }}
      >
        <text x={8} y={TOP_PAD + ROW_H / 2} fontSize={12} fill="#88a">
          {selectedChannel}
        </text>
        <rect x={LABEL_W} y={TOP_PAD} width={plotW} height={ROW_H} fill="#0d1420" stroke="#243" />
        {gaps.map((g, i) => (
          <rect key={i} x={LABEL_W + g.x} y={TOP_PAD} width={g.w} height={ROW_H} fill="#3a2a12" opacity={0.7} />
        ))}
        <polyline
          points={line}
          fill="none"
          stroke="#7ee787"
          strokeWidth={1.5}
          transform={`translate(${LABEL_W},0)`}
        />
        {visibleItems.map((it, i) => (
          <ItemMarker key={i} item={it} vp={vp} plotW={plotW} />
        ))}
        {cursor !== null && (
          <line
            x1={LABEL_W + nsToPx(cursor, plotW, vp)}
            x2={LABEL_W + nsToPx(cursor, plotW, vp)}
            y1={TOP_PAD}
            y2={TOP_PAD + ROW_H}
            stroke="#79c0ff"
            strokeDasharray="3 3"
          />
        )}
      </svg>
      <div className="vpbar">
        <span>
          窗口 {formatNs(vp.startNs)} .. {formatNs(vp.startNs + vp.durationNs)}
        </span>
        <span>{cursor !== null ? `光标 ${formatNs(cursor)}` : "滚轮以指针时刻为锚点缩放，拖拽平移"}</span>
      </div>
    </div>
  );
}

function ItemMarker({ item, vp, plotW }: { item: Item; vp: Viewport; plotW: number }) {
  const x1 = LABEL_W + nsToPx(item.startNs, plotW, vp);
  const x2 = LABEL_W + nsToPx(item.endNs, plotW, vp);
  const w = Math.max(3, x2 - x1);
  const isErr = item.kind === "error";
  const label = isErr
    ? `ERR:${item.errCode}`
    : item.value !== undefined
      ? `0x${item.value.toString(16).toUpperCase().padStart(2, "0")}`
      : "";
  return (
    <g>
      <rect
        x={x1}
        y={TOP_PAD - 14}
        width={w}
        height={12}
        fill={isErr ? "#5c1f24" : "#143a5c"}
        stroke={isErr ? "#ff7b72" : "#58a6ff"}
      >
        <title>{`${label}\n${formatNs(item.startNs)} .. ${formatNs(item.endNs)}\n${item.message ?? ""}`}</title>
      </rect>
      {w > 26 && (
        <text x={x1 + 3} y={TOP_PAD - 5} fontSize={9} fill={isErr ? "#ffb3ad" : "#b6e3ff"}>
          {label}
        </text>
      )}
    </g>
  );
}
