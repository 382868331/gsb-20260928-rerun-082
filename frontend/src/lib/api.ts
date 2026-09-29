import type { Capture, Chunk, Config, Result } from '../types';

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!response.ok) throw new Error((await response.json()).error ?? response.statusText);
  return response.json();
}

export const api = {
  demo: () => request<{ capture: Capture; chunks: Chunk[]; config: Config; result: Result }>('/api/demo', { method: 'POST' }),
  captures: () => request<Capture[]>('/api/captures'),
  capture: (id: string) => request<{ capture: Capture; chunks: Chunk[] }>(`/api/captures/${id}`),
  createConfig: (captureId: string, parameters: Config['parameters'], name: string) =>
    request<{ config: Config; result: Result }>(`/api/captures/${captureId}/configs`, {
      method: 'POST',
      body: JSON.stringify({ protocol: 'uart', name, parameters }),
    }),
  configs: (captureId: string) => request<Config[]>(`/api/captures/${captureId}/configs`),
  results: (captureId: string) => request<Result[]>(`/api/captures/${captureId}/results`),
};
