export interface SampleConfig {
  version: string;
  channels: string[];
  periodNs: number;
  createdAt?: string;
  comment?: string;
}

export interface SampleBlock {
  configVersion: string;
  startIndex: number;
  count: number;
  levels: string; // base64 packed levels bytes
}

export interface DecodeConfig {
  version: string;
  protocol: string;
  channels: string[];
  baudRate: number;
  dataBits?: number;
  parity?: string;
  stopBits?: number;
  invert?: boolean;
  clockTolerance?: number;
  createdAt?: string;
  comment?: string;
}

export interface Bit {
  name: string;
  level: number;
  atNs: number;
}

export interface Item {
  kind: "data" | "error";
  startNs: number;
  endNs: number;
  channel: string;
  value?: number;
  bits?: Bit[];
  errCode?: string;
  message?: string;
}

export interface DecodeResult {
  version: string;
  sampleConfigVersion: string;
  decodeConfigVersion: string;
  protocol: string;
  items: Item[];
  bytes: number[];
  errorCount: number;
  computedAt: string;
}

export interface ChangedItem {
  startNs: number;
  a: Item;
  b: Item;
}

export interface Diff {
  a: string;
  b: string;
  same: boolean;
  onlyInA: Item[];
  onlyInB: Item[];
  changed: ChangedItem[];
  byteStreamA: number[];
  byteStreamB: number[];
}
