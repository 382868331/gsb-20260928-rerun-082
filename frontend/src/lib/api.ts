import type { Alignment, AlignmentAnchor, AlignmentEstimate, AnchorCandidate, Capture, Chunk, Config, CrossReport, DemoCapture, DiffReport, Result, SPIParameters, UARTParameters } from '../types';

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
  alignments: () => request<Alignment[]>('/api/alignments'),
  createAlignment: (name: string, baseCaptureId: string, otherCaptureId: string, anchors: AlignmentAnchor[]) =>
    request<Alignment>('/api/alignments', {
      method: 'POST',
      body: JSON.stringify({ name, baseCaptureId, otherCaptureId, anchors }),
    }),
  alignment: (groupId: string, version?: number) =>
    request<{ alignment: Alignment; versions: Alignment[]; estimate: AlignmentEstimate }>(
      `/api/alignments/${groupId}${version ? `?version=${version}` : ''}`,
    ),
  addAlignmentVersion: (groupId: string, anchors: AlignmentAnchor[]) =>
    request<{ alignment: Alignment; estimate: AlignmentEstimate }>(`/api/alignments/${groupId}/versions`, {
      method: 'POST',
      body: JSON.stringify({ anchors }),
    }),
  alignmentCandidates: (groupId: string, baseResultId: string, otherResultId: string) =>
    request<{ accepted: AnchorCandidate[]; excluded: AnchorCandidate[] }>(`/api/alignments/${groupId}/candidates`, {
      method: 'POST',
      body: JSON.stringify({ baseResultId, otherResultId }),
    }),
  compareAlignment: (groupId: string, version: number, baseResultId: string, otherResultId: string) =>
    request<CrossReport>(`/api/alignments/${groupId}/compare`, {
      method: 'POST',
      body: JSON.stringify({ version, baseResultId, otherResultId }),
    }),
};
