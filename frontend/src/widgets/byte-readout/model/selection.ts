import type { TcpSession } from '@/entities/session';
import type { StreamSegment } from '@/entities/stream';
import type { Direction, SourceRef } from '@/shared/api';
import { maskHasAny, rangeEnd, toHexMasked, type ByteRange } from '@/shared/lib';

export interface SelectionDetails {
  /** Provenance of the bytes: capture, session, direction, range and the packets that carry them. */
  source: SourceRef;
  /** Hex of the selection, unknown bytes as "??". */
  hex: string;
  hasUnknown: boolean;
}

/** Packets whose payload overlaps the range. */
export function packetsOf(segments: readonly StreamSegment[], range: ByteRange): number[] {
  return segments.filter((s) => s.offset < rangeEnd(range) && range.offset < s.offset + s.length).map((s) => s.packetNumber);
}

export function describeSelection(
  session: TcpSession,
  direction: Direction,
  bytes: Uint8Array,
  unknownMask: Uint8Array,
  segments: readonly StreamSegment[],
  selection: ByteRange,
): SelectionDetails {
  const end = rangeEnd(selection);
  return {
    source: {
      captureId: session.captureId,
      captureSha256: session.capture.sha256,
      sessionId: session.id,
      direction,
      offset: selection.offset,
      length: selection.length,
      packetNumbers: packetsOf(segments, selection),
    },
    hex: toHexMasked(bytes.subarray(selection.offset, end), unknownMask.subarray(selection.offset, end)),
    hasUnknown: maskHasAny(unknownMask, selection),
  };
}
