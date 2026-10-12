import type { FramingRule } from '@/entities/interpretation';
import type { Message } from '@/entities/message';
import type { StreamGap, StreamSegment } from '@/entities/stream';
import { parseHex } from '@/shared/lib';

export interface LaneSegment {
  key: string;
  label: string;
  /** Message-relative byte range. */
  offset: number;
  length: number;
  /** Field tone of the "Поля" lane (0..3); the other lanes are neutral. */
  tone?: number;
  kind?: 'gap' | 'violation';
  /** Path of the field this segment stands for (links the lane, the table and the hex view). */
  path?: string;
  title: string;
}

/** TCP lane: the segments that carry the message, cut to its bounds, and the capture gaps between them. */
export function tcpLane(message: Message, segments: readonly StreamSegment[], gaps: readonly StreamGap[]): LaneSegment[] {
  const from = message.offset;
  const to = message.offset + message.length;
  const out: LaneSegment[] = [];
  for (const s of segments) {
    const a = Math.max(from, s.offset);
    const b = Math.min(to, s.offset + s.length);
    if (b <= a) continue;
    out.push({
      key: `p${s.packetNumber}@${s.offset}`,
      label: `#${s.packetNumber}  ${b - a} Б`,
      offset: a - from,
      length: b - a,
      title: `Пакет #${s.packetNumber}, ${b - a} Б${s.flags.length ? `, ${s.flags.join(', ')}` : ''}`,
    });
  }
  for (const g of gaps) {
    const a = Math.max(from, g.offset);
    const b = Math.min(to, g.offset + g.length);
    if (b <= a) continue;
    out.push({ key: `g${g.offset}`, label: `потеря ${b - a} Б`, offset: a - from, length: b - a, kind: 'gap', title: `Пропуск в записи, ${b - a} Б` });
  }
  return out.sort((x, y) => x.offset - y.offset);
}

/** Frame lane: header, data and trailer as the framing rule of the interpretation cuts the message. */
export function frameLane(framing: FramingRule | undefined, length: number): LaneSegment[] {
  if (!framing || length <= 0) return [];
  const seg = (key: string, label: string, offset: number, len: number): LaneSegment => ({
    key,
    label: `${label} ${len}`,
    offset,
    length: len,
    title: `${label}: ${len} Б со смещения ${offset}`,
  });

  if (framing.kind === 'length_prefixed') {
    const header = Math.min(length, framing.lengthField.offset + framing.lengthField.size);
    const trailer = Math.min(length - header, Math.max(0, framing.lengthAdjust - header));
    const data = length - header - trailer;
    return [
      seg('head', 'заголовок', 0, header),
      ...(data > 0 ? [seg('data', 'данные', header, data)] : []),
      ...(trailer > 0 ? [seg('tail', 'хвост', header + data, trailer)] : []),
    ];
  }
  if (framing.kind === 'delimiter') {
    const delimiter = Math.min(length, parseHex(framing.delimiter)?.length ?? 1);
    return [
      ...(length > delimiter ? [seg('data', 'данные', 0, length - delimiter)] : []),
      seg('delim', 'разделитель', length - delimiter, delimiter),
    ];
  }
  return [seg('frame', 'кадр', 0, length)];
}

/** Field lane: the top-level fields of the message; arrays are one segment. */
export function fieldLane(message: Message): LaneSegment[] {
  return message.fields.map((f, i) => ({
    key: f.path,
    label: f.children?.length ? `${f.name}  ${f.display}` : f.name,
    offset: f.offset,
    length: f.length,
    tone: i,
    kind: f.status === 'violation' ? 'violation' : undefined,
    path: f.path,
    title: `${f.path}: @${f.offset}, ${f.length} Б, ${f.type}${f.display ? ` = ${f.display}` : ''}`,
  }));
}

const STEPS = [1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536];

/** Offsets to label on the lane ruler: a round step that keeps labels apart, plus the last byte. */
export function rulerTicks(length: number, widthPx: number, minGapPx = 48): number[] {
  if (length <= 1 || widthPx <= 0) return [0];
  const pxPerByte = widthPx / length;
  const step = STEPS.find((s) => s * pxPerByte >= minGapPx) ?? STEPS[STEPS.length - 1];
  const ticks: number[] = [];
  for (let o = 0; o < length; o += step) ticks.push(o);
  const last = length - 1;
  if (last * pxPerByte - ticks[ticks.length - 1] * pxPerByte >= minGapPx * 0.6) ticks.push(last);
  else if (ticks.length > 1) ticks[ticks.length - 1] = last;
  return ticks;
}
