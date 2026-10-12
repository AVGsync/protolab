import type { Diagnostic, Endpoint, ISODateTime } from '@/shared/api';

export type SessionStatus = 'complete' | 'partial' | 'error';
/** How client/server roles were determined. */
export type RoleCertainty = 'handshake' | 'inferred' | 'unknown';

export interface SessionFlags {
  handshake: boolean;
  closed: boolean;
  gaps: number;
  retransmissions: number;
  outOfOrder: number;
  overlapConflicts: number;
  checksumWarnings: number;
}

/**
 * One TCP connection instance. Reused address/port tuples produce distinct
 * sessions — they are never merged.
 */
export interface TcpSession {
  id: string;
  projectId: string;
  captureId: string;
  capture: { fileName: string; sha256: string };
  /** Ordinal within the capture (Wireshark tcp.stream). */
  index: number;
  client: Endpoint;
  server: Endpoint;
  roles: RoleCertainty;
  startedAt: ISODateTime;
  endedAt: ISODateTime;
  packetCount: number;
  bytes: { c2s: number; s2c: number };
  status: SessionStatus;
  flags: SessionFlags;
  /** Server port is one of the project's target ports. */
  isTarget: boolean;
  diagnostics: Diagnostic[];
  actionIds: string[];
}

export interface SessionFilters {
  captureId?: string;
  status?: SessionStatus;
  target?: boolean;
  q?: string;
}
