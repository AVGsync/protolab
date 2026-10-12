import type { Direction, ISODateTime } from '@/shared/api';

export type SegmentFlag = 'retransmission' | 'out_of_order' | 'overlap_conflict' | 'checksum_warning' | 'truncated' | 'keep_alive';

/** A TCP payload placed into the directed stream by its sequence number. */
export interface StreamSegment {
  offset: number;
  length: number;
  packetNumber: number;
  timestamp: ISODateTime;
  flags: SegmentFlag[];
}

/** A missed beginning of the stream is not a gap: see DirectedStream.startKnown. */
export type GapReason = 'lost' | 'truncated';

/** Bytes that are known to exist but were not captured. Never substituted by zeros. */
export interface StreamGap {
  offset: number;
  length: number;
  reason: GapReason;
}

/** Retransmission carrying different bytes for the same sequence range. */
export interface OverlapConflict {
  offset: number;
  length: number;
  packetNumbers: number[];
  /** Which copy was used in the reassembled stream. */
  chosenPacket: number;
}

/** Reassembled bytes of one direction (metadata; bytes are fetched separately). */
export interface DirectedStream {
  sessionId: string;
  direction: Direction;
  length: number;
  /** false when the beginning of the stream was not captured. */
  startKnown: boolean;
  segments: StreamSegment[];
  gaps: StreamGap[];
  conflicts: OverlapConflict[];
}

export interface Packet {
  /** 1-based packet number in the capture. */
  number: number;
  timestamp: ISODateTime;
  direction: Direction;
  tcpFlags: string[];
  /** Relative sequence number. */
  seq: number;
  ack: number;
  payloadLength: number;
  /** Offset of the payload in the directed stream; null for pure ACKs and duplicates. */
  streamOffset: number | null;
  flags: SegmentFlag[];
}
