import type { Capture } from '@/entities/capture';
import type { TcpSession } from '@/entities/session';
import type { DirectedStream, OverlapConflict, Packet, SegmentFlag, StreamGap } from '@/entities/stream';
import type { Diagnostic, Direction, Endpoint } from '@/shared/api';
import { fakeSha256 } from './hash';
import { isoAt, MS } from './time';

/* -------------------------------------------------------------------------- */
/*  Scenario description                                                      */
/* -------------------------------------------------------------------------- */

export interface ExchangeSpec {
  req: Uint8Array;
  resp?: Uint8Array;
  /** Split points (byte offsets) of the request/response into TCP segments. */
  reqSplit?: number[];
  respSplit?: number[];
  /** Pause before the exchange, µs. */
  pauseUs?: number;
}

export interface DefectSpec {
  drop?: { dir: Direction; index: number };
  truncate?: { dir: Direction; index: number; keep: number };
  retransmit?: { dir: Direction; index: number; conflict?: boolean };
  reorder?: { dir: Direction; index: number };
  /** Capture started after this many packets of the connection (no handshake). */
  dropLeading?: number;
  checksumWarnings?: Direction;
  noClose?: boolean;
}

export interface SessionSpec {
  key: string;
  client: Endpoint;
  server: Endpoint;
  /** Offset from capture start, µs. */
  startUs: number;
  exchanges: ExchangeSpec[];
  defects?: DefectSpec;
  actionId?: string;
}

export interface CaptureSpec {
  id: string;
  projectId: string;
  fileName: string;
  format: 'pcap' | 'pcapng';
  startIso: string;
  importedAt: string;
  snapLen: number;
  targetPorts: number[];
  sessions: SessionSpec[];
  /** Packets that are not TCP/IPv4 (ARP, mDNS…) — skipped with a diagnostic. */
  nonTcpPackets?: number;
}

/* -------------------------------------------------------------------------- */
/*  Output                                                                    */
/* -------------------------------------------------------------------------- */

export interface StreamData {
  meta: DirectedStream;
  /** Reassembled bytes; gap bytes are zero-filled placeholders. */
  bytes: Uint8Array;
}

export interface GeneratedSession {
  session: TcpSession;
  packets: Packet[];
  streams: Record<Direction, StreamData>;
}

export interface GeneratedCapture {
  capture: Capture;
  sessions: GeneratedSession[];
}

/* -------------------------------------------------------------------------- */

interface RawPacket {
  tUs: number;
  dir: Direction;
  tcpFlags: string[];
  seq: number;
  ack: number;
  payload: Uint8Array | null;
  /** Offset in the directed stream; null for pure ACKs and retransmitted copies. */
  streamOffset: number | null;
  flags: SegmentFlag[];
  captured: boolean;
  number: number;
}

const other = (d: Direction): Direction => (d === 'c2s' ? 's2c' : 'c2s');

function split(bytes: Uint8Array, points: number[] = []): Uint8Array[] {
  const cuts = [0, ...points.filter((p) => p > 0 && p < bytes.length).sort((a, b) => a - b), bytes.length];
  const out: Uint8Array[] = [];
  for (let i = 0; i < cuts.length - 1; i++) out.push(bytes.subarray(cuts[i], cuts[i + 1]));
  return out;
}

interface BuiltSession {
  spec: SessionSpec;
  raw: RawPacket[];
  data: Record<Direction, RawPacket[]>;
  chunks: Record<Direction, Uint8Array>;
  gaps: Record<Direction, StreamGap[]>;
  conflicts: Record<Direction, { offset: number; length: number; orig: RawPacket; dup: RawPacket }[]>;
  handshake: boolean;
  closed: boolean;
}

function buildRaw(spec: SessionSpec): BuiltSession {
  const raw: RawPacket[] = [];
  const data: Record<Direction, RawPacket[]> = { c2s: [], s2c: [] };
  const parts: Record<Direction, Uint8Array[]> = { c2s: [], s2c: [] };
  const cursor: Record<Direction, number> = { c2s: 0, s2c: 0 };
  let t = spec.startUs;

  const push = (dir: Direction, tcpFlags: string[], payload: Uint8Array | null, dtUs: number, seq?: number) => {
    t += dtUs;
    const p: RawPacket = {
      tUs: t,
      dir,
      tcpFlags,
      seq: seq ?? 1 + cursor[dir],
      ack: 1 + cursor[other(dir)],
      payload,
      streamOffset: payload ? cursor[dir] : null,
      flags: [],
      captured: true,
      number: 0,
    };
    if (payload) {
      parts[dir].push(payload);
      cursor[dir] += payload.length;
      data[dir].push(p);
    }
    raw.push(p);
    return p;
  };

  push('c2s', ['SYN'], null, 0, 0);
  push('s2c', ['SYN', 'ACK'], null, 310, 0);
  push('c2s', ['ACK'], null, 180);

  for (const ex of spec.exchanges) {
    t += ex.pauseUs ?? 40 * MS;
    split(ex.req, ex.reqSplit).forEach((piece, i) => push('c2s', ['PSH', 'ACK'], piece, i === 0 ? 120 : 95));
    push('s2c', ['ACK'], null, 260);
    if (ex.resp) {
      split(ex.resp, ex.respSplit).forEach((piece, i) => push('s2c', ['PSH', 'ACK'], piece, i === 0 ? 3800 : 140));
      push('c2s', ['ACK'], null, 210);
    }
  }

  const closed = !spec.defects?.noClose;
  if (closed) {
    push('c2s', ['FIN', 'ACK'], null, 15 * MS);
    push('s2c', ['FIN', 'ACK'], null, 400);
    push('c2s', ['ACK'], null, 150);
  }

  const chunks = {
    c2s: concatAll(parts.c2s),
    s2c: concatAll(parts.s2c),
  };

  return {
    spec,
    raw,
    data,
    chunks,
    gaps: { c2s: [], s2c: [] },
    conflicts: { c2s: [], s2c: [] },
    handshake: true,
    closed,
  };
}

function concatAll(parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let pos = 0;
  for (const p of parts) {
    out.set(p, pos);
    pos += p.length;
  }
  return out;
}

function applyDefects(b: BuiltSession): void {
  const d = b.spec.defects;
  if (!d) return;

  if (d.retransmit) {
    const orig = b.data[d.retransmit.dir][d.retransmit.index];
    const payload = orig.payload!.slice();
    if (d.retransmit.conflict) payload[Math.floor(payload.length / 2)] ^= 0x5a;
    const dup: RawPacket = {
      ...orig,
      tUs: orig.tUs + 1400,
      payload,
      streamOffset: null,
      flags: [d.retransmit.conflict ? 'overlap_conflict' : 'retransmission'],
    };
    b.raw.push(dup);
    if (d.retransmit.conflict) {
      b.conflicts[d.retransmit.dir].push({ offset: orig.streamOffset!, length: payload.length, orig, dup });
    }
  }

  if (d.reorder) {
    const list = b.data[d.reorder.dir];
    const first = list[d.reorder.index];
    const second = list[d.reorder.index + 1];
    [first.tUs, second.tUs] = [second.tUs, first.tUs];
    first.flags.push('out_of_order');
  }

  if (d.drop) {
    const p = b.data[d.drop.dir][d.drop.index];
    p.captured = false;
    b.gaps[d.drop.dir].push({ offset: p.streamOffset!, length: p.payload!.length, reason: 'lost' });
  }

  if (d.truncate) {
    const p = b.data[d.truncate.dir][d.truncate.index];
    p.flags.push('truncated');
    b.gaps[d.truncate.dir].push({
      offset: p.streamOffset! + d.truncate.keep,
      length: p.payload!.length - d.truncate.keep,
      reason: 'truncated',
    });
  }

  if (d.dropLeading) {
    [...b.raw]
      .sort((x, y) => x.tUs - y.tUs)
      .slice(0, d.dropLeading)
      .forEach((p) => (p.captured = false));
    b.handshake = false;
  }

  if (d.checksumWarnings) {
    for (const p of b.raw) if (p.dir === d.checksumWarnings && p.captured) p.flags.push('checksum_warning');
  }
}

interface SessionCtx {
  capture: Pick<Capture, 'id' | 'projectId' | 'fileName' | 'sha256'>;
  startIso: string;
  targetPorts: number[];
  index: number;
  sessionId: string;
}

function finalizeSession(b: BuiltSession, ctx: SessionCtx): GeneratedSession {
  const ts = (p: RawPacket) => isoAt(ctx.startIso, p.tUs);
  const streams = {} as Record<Direction, StreamData>;

  for (const dir of ['c2s', 's2c'] as const) {
    const captured = b.data[dir].filter((p) => p.captured);
    // When the beginning of the connection was not captured the stream starts
    // at the first byte we actually saw.
    const base = b.handshake ? 0 : captured.length ? Math.min(...captured.map((p) => p.streamOffset!)) : 0;
    const bytes = b.chunks[dir].slice(base);
    const gaps = b.gaps[dir]
      .filter((g) => g.offset + g.length > base)
      .map((g) => ({ ...g, offset: g.offset - base }));
    for (const g of gaps) bytes.fill(0, g.offset, g.offset + g.length);

    const conflicts: OverlapConflict[] = b.conflicts[dir].map((c) => ({
      offset: c.offset - base,
      length: c.length,
      packetNumbers: [c.orig.number, c.dup.number],
      chosenPacket: c.orig.number,
    }));

    streams[dir] = {
      bytes,
      meta: {
        sessionId: ctx.sessionId,
        direction: dir,
        length: bytes.length,
        startKnown: b.handshake,
        segments: captured
          .map((p) => ({
            offset: p.streamOffset! - base,
            length: p.payload!.length,
            packetNumber: p.number,
            timestamp: ts(p),
            flags: [...p.flags],
          }))
          .sort((x, y) => x.offset - y.offset),
        gaps,
        conflicts,
      },
    };

    for (const p of b.data[dir]) if (p.streamOffset !== null) p.streamOffset -= base;
  }

  const captured = b.raw.filter((p) => p.captured).sort((x, y) => x.tUs - y.tUs);
  const packets: Packet[] = captured.map((p) => ({
    number: p.number,
    timestamp: ts(p),
    direction: p.dir,
    tcpFlags: p.tcpFlags,
    seq: p.seq,
    ack: p.ack,
    payloadLength: p.payload?.length ?? 0,
    streamOffset: p.streamOffset,
    flags: [...p.flags],
  }));

  const count = (flag: SegmentFlag) => captured.filter((p) => p.flags.includes(flag)).length;
  const gapsTotal = streams.c2s.meta.gaps.length + streams.s2c.meta.gaps.length;
  const conflictsTotal = streams.c2s.meta.conflicts.length + streams.s2c.meta.conflicts.length;

  const diagnostics: Diagnostic[] = [];
  for (const dir of ['c2s', 's2c'] as const) {
    for (const g of streams[dir].meta.gaps) {
      diagnostics.push({
        level: 'warning',
        code: g.reason === 'truncated' ? 'tcp_truncated' : 'tcp_gap',
        message: `${dir === 'c2s' ? 'C→S' : 'S→C'}: ${g.length} Б неизвестны на смещении 0x${g.offset
          .toString(16)
          .toUpperCase()} (${g.reason === 'truncated' ? 'пакет усечён snaplen' : 'потерян сегмент'})`,
        sessionId: ctx.sessionId,
      });
    }
    for (const c of streams[dir].meta.conflicts) {
      diagnostics.push({
        level: 'error',
        code: 'overlap_conflict',
        message: `${dir === 'c2s' ? 'C→S' : 'S→C'}: повторная передача с другими байтами (${c.length} Б), использован пакет #${c.chosenPacket}`,
        sessionId: ctx.sessionId,
        packetNumbers: c.packetNumbers,
      });
    }
  }
  if (!b.handshake) {
    diagnostics.push({
      level: 'warning',
      code: 'missing_handshake',
      message: 'Начало соединения не захвачено: роли определены по порту, первые байты потока неизвестны',
      sessionId: ctx.sessionId,
    });
  }
  const checksum = count('checksum_warning');
  if (checksum) {
    diagnostics.push({
      level: 'info',
      code: 'checksum_offload',
      message: 'Неверная контрольная сумма TCP — вероятно checksum offloading на узле захвата; данные используются',
      sessionId: ctx.sessionId,
      count: checksum,
    });
  }

  const status = b.handshake && b.closed && gapsTotal === 0 && conflictsTotal === 0 ? 'complete' : 'partial';
  const isTarget = ctx.targetPorts.includes(b.spec.server.port);

  return {
    packets,
    streams,
    session: {
      id: ctx.sessionId,
      projectId: ctx.capture.projectId,
      captureId: ctx.capture.id,
      capture: { fileName: ctx.capture.fileName, sha256: ctx.capture.sha256 },
      index: ctx.index,
      client: b.spec.client,
      server: b.spec.server,
      roles: b.handshake ? 'handshake' : isTarget ? 'inferred' : 'unknown',
      startedAt: packets[0]?.timestamp ?? ctx.startIso,
      endedAt: packets[packets.length - 1]?.timestamp ?? ctx.startIso,
      packetCount: packets.length,
      bytes: { c2s: streams.c2s.bytes.length, s2c: streams.s2c.bytes.length },
      status,
      flags: {
        handshake: b.handshake,
        closed: b.closed,
        gaps: gapsTotal,
        retransmissions: count('retransmission'),
        outOfOrder: count('out_of_order'),
        overlapConflicts: conflictsTotal,
        checksumWarnings: checksum,
      },
      isTarget,
      diagnostics,
      actionIds: b.spec.actionId ? [b.spec.actionId] : [],
    },
  };
}

/** Builds a capture: packets of all sessions are interleaved by time and numbered. */
export function generateCapture(spec: CaptureSpec, sessionId: (key: string, index: number) => string): GeneratedCapture {
  const built = spec.sessions.map((s) => {
    const b = buildRaw(s);
    applyDefects(b);
    return b;
  });

  const all = built.flatMap((b) => b.raw.filter((p) => p.captured)).sort((x, y) => x.tUs - y.tUs);
  // Non-TCP packets are interleaved into numbering but not into sessions.
  const skipEvery = spec.nonTcpPackets ? Math.max(2, Math.floor(all.length / spec.nonTcpPackets)) : 0;
  let n = 0;
  all.forEach((p, i) => {
    n += 1;
    if (skipEvery && i % skipEvery === skipEvery - 1) n += 1;
    p.number = n;
  });
  const packetCount = n;

  const sha256 = fakeSha256(`${spec.fileName}:${packetCount}`);
  const capture: Capture = {
    id: spec.id,
    projectId: spec.projectId,
    fileName: spec.fileName,
    format: spec.format,
    sizeBytes: 0,
    sha256,
    integrity: 'ok',
    linkType: 'EN10MB',
    snapLen: spec.snapLen,
    interfaces: 1,
    packetCount,
    sessionCount: built.length,
    targetSessionCount: built.filter((b) => spec.targetPorts.includes(b.spec.server.port)).length,
    firstPacketAt: all.length ? isoAt(spec.startIso, all[0].tUs) : null,
    lastPacketAt: all.length ? isoAt(spec.startIso, all[all.length - 1].tUs) : null,
    importedAt: spec.importedAt,
    status: 'ready',
    diagnostics: [],
  };

  const sessions = built.map((b, i) =>
    finalizeSession(b, {
      capture,
      startIso: spec.startIso,
      targetPorts: spec.targetPorts,
      index: i,
      sessionId: sessionId(b.spec.key, i),
    }),
  );

  const header = spec.format === 'pcapng' ? 200 : 24;
  capture.sizeBytes =
    header +
    sessions.reduce((sum, s) => sum + s.packets.reduce((acc, p) => acc + 16 + 54 + p.payloadLength, 0), 0) +
    (spec.nonTcpPackets ?? 0) * 76;

  if (spec.nonTcpPackets) {
    capture.diagnostics.push({
      level: 'info',
      code: 'non_tcp_skipped',
      message: 'Пакеты вне TCP/IPv4 (ARP, mDNS) пропущены',
      count: spec.nonTcpPackets,
    });
  }
  const bySession = sessions.flatMap((s) => s.session.diagnostics);
  const codes = new Map<string, Diagnostic>();
  for (const d of bySession) {
    const existing = codes.get(d.code);
    if (existing) existing.count = (existing.count ?? 1) + (d.count ?? 1);
    else codes.set(d.code, { level: d.level, code: d.code, message: CAPTURE_LEVEL_MESSAGE[d.code] ?? d.message, count: d.count ?? 1 });
  }
  capture.diagnostics.push(...codes.values());

  return { capture, sessions };
}

const CAPTURE_LEVEL_MESSAGE: Record<string, string> = {
  tcp_gap: 'Потерянные TCP-сегменты: неизвестные участки помечены, нулями не подменяются',
  tcp_truncated: 'Пакеты усечены snaplen: хвосты полезной нагрузки неизвестны',
  overlap_conflict: 'Повторные передачи с различающимися данными (политика: первый пакет)',
  missing_handshake: 'Соединения без начала: роли сторон не установлены по рукопожатию',
  checksum_offload: 'Неверные контрольные суммы TCP (вероятно checksum offloading)',
};
