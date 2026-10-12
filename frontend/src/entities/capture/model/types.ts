import type { Diagnostic, ISODateTime } from '@/shared/api';

export type CaptureFormat = 'pcap' | 'pcapng';
export type CaptureStatus = 'indexing' | 'ready' | 'error';
/** Result of comparing the stored file with the SHA-256 recorded on import. */
export type CaptureIntegrity = 'ok' | 'mismatch' | 'unchecked';

/** Immutable source capture. The original file is stored as-is. */
export interface Capture {
  id: string;
  projectId: string;
  fileName: string;
  format: CaptureFormat;
  sizeBytes: number;
  sha256: string;
  integrity: CaptureIntegrity;
  /** libpcap link type name, e.g. "EN10MB". */
  linkType: string;
  snapLen: number;
  interfaces: number;
  packetCount: number;
  sessionCount: number;
  targetSessionCount: number;
  firstPacketAt: ISODateTime | null;
  lastPacketAt: ISODateTime | null;
  importedAt: ISODateTime;
  status: CaptureStatus;
  jobId?: string;
  diagnostics: Diagnostic[];
}

export interface UploadCaptureResult {
  capture: Capture;
  jobId: string;
}
