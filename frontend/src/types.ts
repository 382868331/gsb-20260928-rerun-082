export type LevelRow = number[];

export interface Capture {
  id: string;
  name: string;
  channelNames: string[];
  createdAt: string;
}

export interface Chunk {
  id: string;
  captureId: string;
  index: number;
  segmentId?: string;
  clockReset?: boolean;
  startTimeNs: number;
  intervalNs: number;
  levels: LevelRow[];
}

export interface UARTParameters {
  channel: number;
  baud: number;
  dataBits: number;
  parity: 'none' | 'even' | 'odd' | 'mark' | 'space';
  stopBits: 1 | 2;
  idleLevel: 0 | 1;
  clockTolerance: number;
}

export interface SPIParameters {
  clockChannel: number;
  dataChannel: number;
  csChannel: number;
  csActive: 0 | 1;
  clockEdge: 'rising' | 'falling';
  bitOrder: 'msb' | 'lsb';
  bitsPerWord: number;
}

export interface Config {
  id: string;
  captureId: string;
  version: number;
  protocol: string;
  name: string;
  parameters: UARTParameters;
  spiParameters?: SPIParameters;
  createdAt: string;
}

export interface Marker {
  id: string;
  kind: 'byte' | 'error';
  code?: string;
  segmentId?: string;
  chunkId?: string;
  startNs: number;
  endNs: number;
  channel: number;
  label: string;
  severity: 'info' | 'error';
  value?: number;
  rawBits?: string;
}

export interface Result {
  id: string;
  captureId: string;
  configId: string;
  configVersion: number;
  protocol: string;
  schemaVersion: number;
  validBytes: number;
  errorCount: number;
  markers: Marker[];
}

export interface DemoCapture {
  capture: Capture;
  chunks: Chunk[];
  configs: Config[];
  results: Result[];
}

export interface MarkerDiff {
  index: number;
  startNs: number;
  endNs: number;
  channel: number;
  base?: Marker;
  other?: Marker;
  cause: 'bit-order' | 'clock-edge' | 'incomplete-frame' | 'other-decode-difference';
  causeText: string;
  detail: string;
}

export interface DiffReport {
  baseResultId: string;
  otherResultId: string;
  identical: boolean;
  diffCount: number;
  firstDiff?: MarkerDiff;
  diffs: MarkerDiff[];
}

export interface AlignmentAnchor {
  id: string;
  baseMarkerId: string;
  otherMarkerId: string;
  sequence?: number;
  baseSegmentId?: string;
  otherSegmentId?: string;
  baseTimeNs: number;
  otherTimeNs: number;
  uncertaintyNs: number;
  manualOrderConfirmed?: boolean;
  evidence?: string;
}

export interface AlignmentCandidate {
  id: string;
  baseMarkerId: string;
  otherMarkerId: string;
  baseTimeNs: number;
  otherTimeNs: number;
  score: number;
  status: 'accepted' | 'excluded';
  reason: string;
  anchorId?: string;
  predictedNs?: number;
  residualNs?: number;
  confidenceHalfWidthNs?: number;
}

export interface Alignment {
  id: string;
  name: string;
  baseCaptureId: string;
  otherCaptureId: string;
  baseResultId: string;
  otherResultId: string;
  anchors: AlignmentAnchor[];
  excludedCandidates: AlignmentCandidate[];
  status: 'ready' | 'insufficient-evidence';
  createdAt: string;
}

export interface AlignmentSegment {
  id?: string;
  baseSegmentId?: string;
  otherSegmentId?: string;
  startOtherNs: number;
  endOtherNs: number;
  offsetNs: number;
  driftRate: number;
}

export interface CrossMarkerDiff {
  base?: Marker;
  other?: Marker;
  baseSegmentId?: string;
  otherSegmentId?: string;
  baseChunkId?: string;
  otherChunkId?: string;
  alignmentId?: string;
  anchorIds?: string[];
  mappedStartNs?: number;
  mappedEndNs?: number;
  otherNativeStartNs?: number;
  otherNativeEndNs?: number;
  confidenceHalfWidthNs: number;
  cause: 'actual-signal' | 'sampling-missing' | 'clock-drift' | 'protocol-parameters' | 'unmatched' | string;
  causeText: string;
  detail: string;
}

export interface CrossDiffReport {
  alignmentId: string;
  baseResultId: string;
  otherResultId: string;
  status: string;
  offsetSummary?: string;
  segments: AlignmentSegment[];
  identical: boolean;
  diffCount: number;
  unmatchedCount: number;
  diffs: CrossMarkerDiff[];
  otherLocations: CrossMarkerDiff[];
}
