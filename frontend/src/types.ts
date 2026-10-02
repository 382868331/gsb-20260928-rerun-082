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
  label?: string;
  baseNs: number;
  otherNs: number;
  source?: string;
}

export interface Alignment {
  id: string;
  groupId: string;
  name: string;
  baseCaptureId: string;
  otherCaptureId: string;
  version: number;
  anchors: AlignmentAnchor[];
  createdAt: string;
}

export interface AnchorOffset {
  anchorId: string;
  label?: string;
  baseNs: number;
  otherNs: number;
  offsetNs: number;
  residualNs: number;
}

export interface AlignmentEstimate {
  anchorCount: number;
  offsetNs: number;
  minOffsetNs: number;
  maxOffsetNs: number;
  driftPpm: number;
  hasDrift: boolean;
  coverageStartNs: number;
  coverageEndNs: number;
  confidenceNs: number;
  anchors: AnchorOffset[];
}

export interface AnchorCandidate {
  value: number;
  baseMarkerId: string;
  otherMarkerId: string;
  baseNs: number;
  otherNs: number;
  offsetNs: number;
  excluded: boolean;
  reason?: string;
}

export interface CrossDiff {
  index: number;
  base?: Marker;
  other?: Marker;
  baseStartNs: number;
  baseEndNs: number;
  otherStartNs?: number;
  otherEndNs?: number;
  confidenceLowNs?: number;
  confidenceHighNs?: number;
  cause: 'signal-difference' | 'sampling-gap' | 'decode-config' | 'unmapped';
  causeText: string;
  detail: string;
}

export interface CrossReport {
  alignmentId: string;
  version: number;
  baseResultId: string;
  otherResultId: string;
  estimate: AlignmentEstimate;
  matched: number;
  diffCount: number;
  diffs: CrossDiff[];
}
