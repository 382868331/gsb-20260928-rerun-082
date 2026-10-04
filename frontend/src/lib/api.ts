import type { Alignment, AlignmentConflict, Capture, Chunk, Config, CrossDiffReport, DemoCapture, DiffReport, Result, SPIParameters, UARTParameters } from '../types';

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!response.ok) throw new Error((await response.json()).error ?? response.statusText);
  return response.json();
}

export const api = {
  demo: () => request<{ captures: DemoCapture[] }>('/api/demo', { method: 'POST' }),
  captures: () => request<Capture[]>('/api/captures'),
  capture: (id: string) => request<{ capture: Capture; chunks: Chunk[] }>(`/api/captures/${id}`),
  createConfig: (captureId: string, protocol: string, name: string, parameters: UARTParameters, spiParameters?: SPIParameters) =>
    request<{ config: Config; result: Result }>(`/api/captures/${captureId}/configs`, {
      method: 'POST',
      body: JSON.stringify({ protocol, name, parameters, spiParameters }),
    }),
  configs: (captureId: string) => request<Config[]>(`/api/captures/${captureId}/configs`),
  results: (captureId: string) => request<Result[]>(`/api/captures/${captureId}/results`),
  diff: (baseResultId: string, otherResultId: string) =>
    request<DiffReport>('/api/results/diff', {
      method: 'POST',
      body: JSON.stringify({ baseResultId, otherResultId }),
    }),
  createAlignment: (baseResultId: string, otherResultId: string, anchors: Array<{ baseMarkerId: string; otherMarkerId: string; uncertaintyNs?: number }>) =>
    request<{ alignment: Alignment; segments: unknown[]; status: string; conflicts?: AlignmentConflict[] }>('/api/alignments', {
      method: 'POST',
      body: JSON.stringify({ name: `跨采样对齐 ${new Date().toLocaleString()}`, baseResultId, otherResultId, anchors }),
    }),
  crossDiff: (alignmentId: string) =>
    request<CrossDiffReport>('/api/results/cross-diff', {
      method: 'POST',
      body: JSON.stringify({ alignmentId }),
    }),
};
