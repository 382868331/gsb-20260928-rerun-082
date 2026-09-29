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

export interface Config {
  id: string;
  captureId: string;
  version: number;
  protocol: string;
  name: string;
  parameters: UARTParameters;
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
