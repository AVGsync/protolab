import type { ByteRange } from '../lib/ranges';

export type { ByteRange };

/** RFC 3339 timestamp with up to nanosecond precision (Go time.RFC3339Nano). */
export type ISODateTime = string;

/** Direction inside a TCP session: client → server or server → client. */
export type Direction = 'c2s' | 's2c';

export interface Endpoint {
  ip: string;
  port: number;
}

/**
 * Provenance of any derived result: the exact bytes of an immutable capture
 * it was obtained from. Must survive interpretation and framing changes.
 */
export interface SourceRef {
  captureId: string;
  /** SHA-256 of the original capture file — guards against silent re-binding. */
  captureSha256: string;
  sessionId: string;
  direction: Direction;
  /** Offset in the reassembled directed stream. */
  offset: number;
  length: number;
  /** Capture packet numbers (1-based, as in Wireshark) covering the range. */
  packetNumbers: number[];
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}

/** Long operations return a job handle; progress is polled via /jobs/{id}. */
export interface JobRef {
  jobId: string;
}

export type DiagnosticLevel = 'info' | 'warning' | 'error';

/** Explicit diagnostic for unsupported, incomplete or ambiguous input data. */
export interface Diagnostic {
  level: DiagnosticLevel;
  /** Stable machine code, e.g. "tcp_gap", "checksum_offload", "unsupported_linktype". */
  code: string;
  message: string;
  sessionId?: string;
  packetNumbers?: number[];
  count?: number;
}
