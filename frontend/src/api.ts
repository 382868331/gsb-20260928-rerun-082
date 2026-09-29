import type {
  DecodeConfig,
  DecodeResult,
  Diff,
  SampleBlock,
  SampleConfig,
} from "./types";

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status} ${path}: ${body}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  listSampleConfigs: () => req<SampleConfig[]>("/api/config/sample"),
  putSampleConfig: (cfg: SampleConfig) =>
    req<SampleConfig>("/api/config/sample", { method: "PUT", body: JSON.stringify(cfg) }),
  listDecodeConfigs: () => req<DecodeConfig[]>("/api/config/decode"),
  putDecodeConfig: (cfg: DecodeConfig) =>
    req<DecodeConfig>("/api/config/decode", { method: "PUT", body: JSON.stringify(cfg) }),
  blocks: (sampleConfig: string) =>
    req<SampleBlock[]>(`/api/blocks?sampleConfig=${encodeURIComponent(sampleConfig)}`),
  appendBlock: (block: SampleBlock) =>
    req<SampleBlock>("/api/blocks", { method: "POST", body: JSON.stringify(block) }),
  decode: (sampleConfig: string, decodeConfig: string) =>
    req<DecodeResult>("/api/decode", {
      method: "POST",
      body: JSON.stringify({ sampleConfig, decodeConfig }),
    }),
  result: (version: string) =>
    req<DecodeResult>(`/api/result?version=${encodeURIComponent(version)}`),
  results: () => req<string[]>("/api/results"),
  compare: (a: string, b: string) =>
    req<Diff>("/api/compare", { method: "POST", body: JSON.stringify({ a, b }) }),
  demo: (params: Record<string, unknown>) =>
    req<{ sampleConfig: SampleConfig; blocks: SampleBlock[] }>("/api/demo", {
      method: "POST",
      body: JSON.stringify(params),
    }),
};
