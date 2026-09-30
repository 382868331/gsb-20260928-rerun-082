import type { Capture, Chunk, Config, DemoCapture, Result, SPIParameters, UARTParameters } from '../types';

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
};
