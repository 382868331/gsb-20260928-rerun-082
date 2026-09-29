import { describe, expect, it } from "vitest";
import { nsToPx, panPx, pxToNs, zoomAt } from "./time";

describe("viewport zoom anchoring", () => {
  const vp = { startNs: 1_000_000, durationNs: 10_000_000 };
  const width = 1000;

  it("keeps the pointer-time at the same screen position after zoom", () => {
    const x = 250;
    const anchor = pxToNs(x, width, vp);
    expect(anchor).toBe(1_000_000 + 0.25 * 10_000_000);

    const z = zoomAt(vp, anchor, 2, 1000, 1_000_000_000, x / width);
    const xAfter = nsToPx(anchor, width, z);
    expect(xAfter).toBeCloseTo(x, 6);
    expect(z.durationNs).toBeCloseTo(5_000_000);
  });

  it("zooms out around the same anchor without losing time position", () => {
    const x = 700;
    const anchor = pxToNs(x, width, vp);
    const z = zoomAt(vp, anchor, 0.5, 1000, 1_000_000_000, x / width);
    expect(nsToPx(anchor, width, z)).toBeCloseTo(x, 6);
    expect(z.durationNs).toBeCloseTo(20_000_000);
  });

  it("panning is reversible", () => {
    const moved = panPx(vp, 120, width);
    const back = panPx(moved, -120, width);
    expect(back.startNs).toBeCloseTo(vp.startNs);
    expect(back.durationNs).toBe(vp.durationNs);
  });
});
