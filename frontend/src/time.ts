// Time/viewport math. The viewport is a half-open nanosecond window
// [startNs, startNs + durationNs). Zooming is anchored to an absolute time
// (usually the instant under the pointer): that instant keeps the same screen
// position after zoom, so the timeline never "jumps" when the page scales.

export interface Viewport {
  startNs: number;
  durationNs: number;
}

export function clamp(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v));
}

/**
 * Zoom while keeping anchorNs fixed at fraction (0..1) of the view width.
 * factor > 1 zooms in, < 1 zooms out. The anchor time may be outside the
 * current window; fraction is still respected.
 */
export function zoomAt(
  vp: Viewport,
  anchorNs: number,
  factor: number,
  minDurationNs: number,
  maxDurationNs: number,
  fraction = 0.5,
): Viewport {
  const newDuration = clamp(vp.durationNs / factor, minDurationNs, maxDurationNs);
  const newStart = anchorNs - fraction * newDuration;
  return { startNs: newStart, durationNs: newDuration };
}

/** Pixel x -> time ns for a viewport of width px. */
export function pxToNs(x: number, width: number, vp: Viewport): number {
  return vp.startNs + (x / width) * vp.durationNs;
}

/** Time ns -> pixel x. */
export function nsToPx(t: number, width: number, vp: Viewport): number {
  return ((t - vp.startNs) / vp.durationNs) * width;
}

/** Pan by a pixel delta. */
export function panPx(vp: Viewport, dxPx: number, width: number): Viewport {
  return { ...vp, startNs: vp.startNs - (dxPx / width) * vp.durationNs };
}

/** Human-readable ns label. */
export function formatNs(ns: number): string {
  if (ns < 1_000) return `${ns.toFixed(0)} ns`;
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(2)} us`;
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(2)} ms`;
  return `${(ns / 1_000_000_000).toFixed(3)} s`;
}
