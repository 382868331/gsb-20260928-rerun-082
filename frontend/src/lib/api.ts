import type { Capture, CaptureBundle, Chunk, Config, Result } from '../types';

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!response.ok) throw new Error((await response.json()).error ?? response.statusText);
  return response.json();
}

export interface NewConfigBody {
  protocol: 'uart' | 'spi';
  name: string;
  parameters?: unknown;
  spiParameters?: unknown;
}

export const api = {
  demo: () => request<{ uart: CaptureBundle; spi: CaptureBundle }>('/api/demo', { method: 'POST' }),
  captures: () => request<Capture[]>('/api/captures'),
  capture: (id: string) => request<{ capture: Capture; chunks: Chunk[] }>(`/api/captures/${id}`),
  createConfig: (captureId: string, body: NewConfigBody) =>
    request<{ config: Config; result: Result }>(`/api/captures/${captureId}/configs`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  configs: (captureId: string) => request<Config[]>(`/api/captures/${captureId}/configs`),
  results: (captureId: string) => request<Result[]>(`/api/captures/${captureId}/results`),
};
