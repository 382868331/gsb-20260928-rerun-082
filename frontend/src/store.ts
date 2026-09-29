import { create } from "zustand";
import { api } from "./api";
import type {
  DecodeConfig,
  DecodeResult,
  Diff,
  SampleBlock,
  SampleConfig,
} from "./types";

interface StationState {
  sampleConfigs: SampleConfig[];
  decodeConfigs: DecodeConfig[];
  blocks: SampleBlock[];
  selectedSample: string | null;
  selectedDecode: string | null;
  result: DecodeResult | null;
  results: Record<string, DecodeResult>;
  compareA: string | null;
  compareB: string | null;
  diff: Diff | null;
  busy: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  loadDemo: (params: Record<string, unknown>) => Promise<void>;
  selectSample: (v: string) => Promise<void>;
  saveDecodeConfig: (cfg: DecodeConfig) => Promise<void>;
  selectDecode: (v: string) => void;
  recompute: () => Promise<void>;
  runCompare: (a: string, b: string) => Promise<void>;
}

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export const useStation = create<StationState>((set, get) => ({
  sampleConfigs: [],
  decodeConfigs: [],
  blocks: [],
  selectedSample: null,
  selectedDecode: null,
  result: null,
  results: {},
  compareA: null,
  compareB: null,
  diff: null,
  busy: false,
  error: null,

  refresh: async () => {
    const [sampleConfigs, decodeConfigs] = await Promise.all([
      api.listSampleConfigs(),
      api.listDecodeConfigs(),
    ]);
    set({ sampleConfigs, decodeConfigs });
    const selectedSample = get().selectedSample;
    if (selectedSample) {
      const blocks = await api.blocks(selectedSample);
      set({ blocks });
    }
  },

  loadDemo: async (params) => {
    set({ busy: true, error: null });
    try {
      const loaded = await api.demo(params);
      await get().refresh();
      set({ selectedSample: loaded.sampleConfig.version, blocks: loaded.blocks, busy: false });
    } catch (e) {
      set({ busy: false, error: String(e) });
    }
  },

  selectSample: async (v) => {
    set({ selectedSample: v, busy: true, error: null });
    try {
      const blocks = await api.blocks(v);
      set({ blocks, busy: false });
    } catch (e) {
      set({ busy: false, error: String(e) });
    }
  },

  saveDecodeConfig: async (cfg) => {
    set({ busy: true, error: null });
    try {
      await api.putDecodeConfig(cfg);
      await get().refresh();
      set({ selectedDecode: cfg.version, busy: false });
    } catch (e) {
      set({ busy: false, error: String(e) });
    }
  },

  selectDecode: (v) => set({ selectedDecode: v }),

  recompute: async () => {
    const { selectedSample, selectedDecode } = get();
    if (!selectedSample || !selectedDecode) return;
    set({ busy: true, error: null });
    try {
      const result = await api.decode(selectedSample, selectedDecode);
      set((s) => ({
        result,
        results: { ...s.results, [result.version]: result },
        busy: false,
      }));
    } catch (e) {
      set({ busy: false, error: String(e) });
    }
  },

  runCompare: async (a, b) => {
    set({ compareA: a, compareB: b, busy: true, error: null });
    try {
      const diff = await api.compare(a, b);
      set({ diff, busy: false });
    } catch (e) {
      set({ busy: false, error: String(e) });
    }
  },
}));

export { b64ToBytes };
