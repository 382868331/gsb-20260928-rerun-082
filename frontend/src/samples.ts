import type { SampleBlock, SampleConfig } from "./types";

// Decode base64 level bytes and answer channel-level queries by global
// sample index. Missing indices (capture gaps between blocks) return null.
export class SampleIndex {
  private ranges: { start: number; end: number; bytes: Uint8Array }[] = [];
  readonly periodNs: number;
  readonly lastIndex: number;

  constructor(blocks: SampleBlock[], cfg: SampleConfig | null) {
    this.periodNs = cfg?.periodNs ?? 1;
    let last = -1;
    for (const b of blocks) {
      const bin = atob(b.levels);
      const bytes = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
      const start = b.startIndex;
      const end = start + b.count - 1;
      this.ranges.push({ start, end, bytes });
      if (end > last) last = end;
    }
    this.lastIndex = last;
  }

  rawByte(globalIdx: number): number | null {
    for (const r of this.ranges) {
      if (globalIdx >= r.start && globalIdx <= r.end) {
        return r.bytes[globalIdx - r.start];
      }
    }
    return null;
  }

  level(globalIdx: number, chanIdx: number): number | null {
    const b = this.rawByte(globalIdx);
    return b === null ? null : (b >> chanIdx) & 1;
  }

  // Contiguous covered intervals in global sample indices, for drawing.
  covered(): { start: number; end: number }[] {
    return this.ranges.map((r) => ({ start: r.start, end: r.end + 1 }));
  }
}
