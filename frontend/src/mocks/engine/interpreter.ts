/**
 * Minimal reference implementation of the "protolab/v1" interpretation engine
 * used by the mock API. The Go backend implements the same semantics; this
 * code exists only so the UI can be developed and demoed without it.
 */
import type { ActionEntry } from '@/entities/action';
import type { FieldRule, InterpretationSpec, MessageRule, SpecIssue, ValidationResult } from '@/entities/interpretation';
import type { Message, MessageStatus, ParsedField } from '@/entities/message';
import type { TcpSession } from '@/entities/session';
import type { StreamGap, StreamSegment } from '@/entities/stream';
import type { Direction } from '@/shared/api';
import { buildMask, bytesToBase64, indexOfAll, parseHex, readInt, toAscii, toHex, type IntEncoding } from '@/shared/lib';

export const ENGINE_VERSION = 'mock-engine/0.3.0';

export interface StreamInput {
  bytes: Uint8Array;
  gaps: StreamGap[];
  segments: StreamSegment[];
}

export interface SessionInput {
  session: TcpSession;
  streams: Record<Direction, StreamInput>;
  /** Actions linked to the session — experiment conditions for assertions. */
  actions: ActionEntry[];
}

interface Frame {
  offset: number;
  length: number;
  issues: string[];
  truncated: boolean;
}

/* -------------------------------------------------------------------------- */
/*  Framing                                                                   */
/* -------------------------------------------------------------------------- */

function frameStream(spec: InterpretationSpec, stream: StreamInput): { frames: Frame[]; unframed: { offset: number; length: number }[] } {
  const { bytes } = stream;
  const unknown = buildMask(bytes.length, stream.gaps);
  const frames: Frame[] = [];
  const unframed: { offset: number; length: number }[] = [];
  const markUnframed = (offset: number, length: number) => {
    const last = unframed[unframed.length - 1];
    if (last && last.offset + last.length === offset) last.length += length;
    else unframed.push({ offset, length });
  };
  const framing = spec.framing;

  if (framing.kind === 'fixed') {
    for (let pos = 0; pos < bytes.length; pos += framing.size) {
      const length = Math.min(framing.size, bytes.length - pos);
      frames.push({ offset: pos, length, issues: length < framing.size ? ['Поток закончился внутри сообщения'] : [], truncated: length < framing.size });
    }
    return { frames, unframed };
  }

  const sync = framing.sync ? parseHex(framing.sync) : null;
  const findSync = (from: number): number => {
    if (!sync) return from;
    for (let i = from; i <= bytes.length - sync.length; i++) {
      if (unknown[i]) continue;
      let ok = true;
      for (let j = 0; j < sync.length; j++) if (bytes[i + j] !== sync[j] || unknown[i + j]) ok = false;
      if (ok) return i;
    }
    return -1;
  };

  if (framing.kind === 'delimiter') {
    const delim = parseHex(framing.delimiter);
    if (!delim) return { frames, unframed };
    let start = 0;
    for (const hit of indexOfAll(bytes, delim)) {
      frames.push({ offset: start, length: hit + delim.length - start, issues: [], truncated: false });
      start = hit + delim.length;
    }
    if (start < bytes.length) frames.push({ offset: start, length: bytes.length - start, issues: ['Нет завершающего разделителя'], truncated: true });
    return { frames, unframed };
  }

  const lf = framing.lengthField;
  const enc = (lf.size === 1 ? 'u8' : lf.size === 2 ? `u16${lf.endian}` : `u32${lf.endian}`) as IntEncoding;
  const minLength = framing.minLength ?? lf.offset + lf.size;
  const maxLength = framing.maxLength ?? 65_536 + 64;

  let pos = 0;
  let needSync = unknown[0] === 1;
  while (pos < bytes.length) {
    if (needSync) {
      const next = findSync(pos);
      if (next < 0) {
        if (pos < bytes.length) markUnframed(pos, bytes.length - pos);
        break;
      }
      if (next > pos) markUnframed(pos, next - pos);
      pos = next;
      needSync = false;
    }

    const headerEnd = pos + lf.offset + lf.size;
    if (headerEnd > bytes.length) {
      frames.push({ offset: pos, length: bytes.length - pos, issues: ['Поток закончился внутри заголовка'], truncated: true });
      break;
    }
    if (unknown.subarray(pos, headerEnd).some((b) => b === 1)) {
      // Header itself is unknown: the length cannot be trusted — resync.
      needSync = true;
      pos += 1;
      while (pos < bytes.length && unknown[pos]) pos++;
      continue;
    }

    const total = readInt(bytes, pos + lf.offset, enc)! + framing.lengthAdjust;
    if (total < minLength || total > maxLength) {
      markUnframed(pos, 1);
      needSync = true;
      pos += 1;
      continue;
    }

    const end = pos + total;
    if (end > bytes.length) {
      frames.push({
        offset: pos,
        length: bytes.length - pos,
        issues: [`Ожидалось ${total} Б, в потоке ${bytes.length - pos} Б`],
        truncated: true,
      });
      break;
    }

    const issues: string[] = [];
    const unknownCount = unknown.subarray(pos, end).reduce((n, b) => n + b, 0);
    if (unknownCount > 0) issues.push(`${unknownCount} Б неизвестны (пропуск в записи)`);
    frames.push({ offset: pos, length: total, issues, truncated: unknownCount > 0 });
    pos = end;
  }

  return { frames, unframed };
}

/* -------------------------------------------------------------------------- */
/*  Field parsing                                                             */
/* -------------------------------------------------------------------------- */

interface ParseCtx {
  msg: Uint8Array;
  unknown: Uint8Array;
  covered: Uint8Array;
}

const INT_TYPES = new Set(['u8', 'i8', 'u16', 'i16', 'u32', 'i32']);

function intEncoding(rule: FieldRule): IntEncoding {
  if (rule.type === 'u8' || rule.type === 'i8') return rule.type;
  return `${rule.type}${rule.endian ?? 'le'}` as IntEncoding;
}

function sizeOf(rule: FieldRule, scope: Map<string, number>): number {
  switch (rule.type) {
    case 'u8':
    case 'i8':
      return 1;
    case 'u16':
    case 'i16':
      return 2;
    case 'u32':
    case 'i32':
      return 4;
    case 'array':
      return (scope.get(rule.count ?? '') ?? 0) * (rule.itemSize ?? 0);
    default:
      return typeof rule.length === 'number' ? rule.length : (scope.get(rule.length ?? '') ?? 0);
  }
}

function round(v: number, scale: number): number {
  const decimals = Math.max(0, Math.ceil(-Math.log10(scale)));
  return Number(v.toFixed(decimals));
}

function parseFields(rules: FieldRule[], ctx: ParseCtx, base: number, pathPrefix: string): ParsedField[] {
  const scope = new Map<string, number>();
  const out: ParsedField[] = [];

  for (const rule of rules) {
    const offset = rule.offset < 0 ? ctx.msg.length + rule.offset : base + rule.offset;
    const length = sizeOf(rule, scope);
    const path = `${pathPrefix}${rule.name}`;
    const field: ParsedField = {
      path,
      name: rule.name,
      offset,
      length,
      type: INT_TYPES.has(rule.type) ? intEncoding(rule) : rule.type,
      value: null,
      display: '',
      status: 'ok',
      meaning: rule.meaning,
      meaningStatus: rule.status ?? (rule.meaning ? 'hypothesis' : 'structural'),
      hypothesisId: rule.hypothesis,
    };
    out.push(field);

    if (offset < 0 || offset + length > ctx.msg.length) {
      field.status = 'violation';
      field.display = '—';
      field.issue = 'Поле выходит за границы сообщения';
      continue;
    }
    ctx.covered.fill(1, offset, offset + length);
    if (ctx.unknown.subarray(offset, offset + length).some((b) => b === 1)) {
      field.status = 'unknown';
      field.display = '??';
      field.issue = 'Байты поля неизвестны (пропуск в записи)';
      continue;
    }

    if (rule.type === 'array') {
      const count = scope.get(rule.count ?? '') ?? 0;
      field.value = count;
      field.display = `${count} × ${rule.itemSize ?? 0} Б`;
      field.children = [];
      for (let i = 0; i < count; i++) {
        const itemBase = offset + i * (rule.itemSize ?? 0);
        const item: ParsedField = {
          path: `${path}[${i}]`,
          name: `[${i}]`,
          offset: itemBase,
          length: rule.itemSize ?? 0,
          type: 'struct',
          value: null,
          display: '',
          status: 'ok',
          children: parseFields(rule.items ?? [], ctx, itemBase, `${path}[${i}].`),
        };
        if (item.children!.some((c) => c.status === 'violation')) item.status = 'violation';
        field.children.push(item);
      }
      if (field.children.some((c) => c.status === 'violation')) field.status = 'violation';
      continue;
    }

    const slice = ctx.msg.subarray(offset, offset + length);
    if (rule.type === 'bytes') {
      field.value = toHex(slice, '');
      field.display = toHex(slice);
      continue;
    }
    if (rule.type === 'ascii') {
      field.value = toAscii(slice);
      field.display = JSON.stringify(field.value);
      continue;
    }

    const raw = readInt(ctx.msg, offset, intEncoding(rule))!;
    scope.set(rule.name, raw);
    field.value = raw;
    const scaled = rule.scale ? round(raw * rule.scale, rule.scale) : raw;
    field.display = rule.enum
      ? `${rule.enum[String(raw)] ?? '?'} (${raw})`
      : `${scaled}${rule.unit ? ` ${rule.unit}` : ''}`;

    if (rule.enum && !(String(raw) in rule.enum)) {
      field.status = 'violation';
      field.issue = `Значение ${raw} отсутствует в перечислении`;
    }
    const e = rule.expect;
    if (e) {
      if (e.equals !== undefined && raw !== e.equals) {
        field.status = 'violation';
        field.issue = `Ожидалось ${e.equals}, получено ${raw}`;
      } else if (e.min !== undefined && raw < e.min) {
        field.status = 'violation';
        field.issue = `Значение ${raw} меньше минимума ${e.min}`;
      } else if (e.max !== undefined && raw > e.max) {
        field.status = 'violation';
        field.issue = `Значение ${raw} больше максимума ${e.max}`;
      } else if (e.oneOf && !e.oneOf.includes(raw)) {
        field.status = 'violation';
        field.issue = `Значение ${raw} не входит в {${e.oneOf.join(', ')}}`;
      }
    }
    if (rule.checksum) {
      const covered = ctx.msg.subarray(0, offset);
      const expected =
        rule.checksum === 'sum8' ? covered.reduce((a, b) => (a + b) & 0xff, 0) : covered.reduce((a, b) => a ^ b, 0);
      if (expected !== raw) {
        field.status = 'violation';
        field.issue = `Контрольная сумма ${rule.checksum}: ожидалось 0x${expected.toString(16).toUpperCase()}`;
      }
    }
  }
  return out;
}

function matchRules(spec: InterpretationSpec, msg: Uint8Array, dir: Direction): MessageRule[] {
  return spec.messages.filter((rule) => {
    if (rule.direction && rule.direction !== 'any' && rule.direction !== dir) return false;
    return rule.match.every((c) => {
      const expected = parseHex(c.hex);
      if (!expected || c.offset + expected.length > msg.length) return false;
      return expected.every((b, i) => msg[c.offset + i] === b);
    });
  });
}

function uncovered(covered: Uint8Array): { offset: number; length: number }[] {
  const out: { offset: number; length: number }[] = [];
  let start = -1;
  for (let i = 0; i <= covered.length; i++) {
    if (i < covered.length && !covered[i]) {
      if (start < 0) start = i;
    } else if (start >= 0) {
      out.push({ offset: start, length: i - start });
      start = -1;
    }
  }
  return out;
}

function getPath(action: ActionEntry, path: string): number | undefined {
  const [root, key] = path.split('.');
  const bag = root === 'params' ? action.params : root === 'observed' ? action.observed : undefined;
  const v = bag?.[key];
  const n = typeof v === 'number' ? v : Number(v);
  return v !== undefined && Number.isFinite(n) ? n : undefined;
}

function applyAssertions(spec: InterpretationSpec, message: Message, rule: MessageRule, actions: ActionEntry[]): void {
  for (const assertion of spec.assertions ?? []) {
    if (assertion.rule !== rule.id) continue;
    const action = actions.find((a) => a.kind === assertion.actionKind);
    if (!action) continue;
    for (const check of assertion.checks) {
      const field = message.fields.find((f) => f.name === check.field);
      const expected = getPath(action, check.equals);
      if (!field || expected === undefined || typeof field.value !== 'number' || field.status === 'unknown') continue;
      const fieldRule = rule.fields.find((f) => f.name === check.field);
      const actual = fieldRule?.scale ? round(field.value * fieldRule.scale, fieldRule.scale) : field.value;
      if (Math.abs(actual - expected) > 1e-6) {
        field.status = 'violation';
        field.issue = `${actual} ≠ ${check.equals} = ${expected} (действие «${action.label}», проверка ${assertion.id})`;
        field.hypothesisId ??= assertion.hypothesis;
      }
    }
  }
}

function timestampAt(segments: StreamSegment[], offset: number): { ts: string; packets: number[] } {
  const seg = segments.find((s) => offset >= s.offset && offset < s.offset + s.length) ?? segments[segments.length - 1];
  return { ts: seg?.timestamp ?? '', packets: seg ? [seg.packetNumber] : [] };
}

function packetsFor(segments: StreamSegment[], offset: number, length: number): number[] {
  return segments
    .filter((s) => s.offset < offset + length && offset < s.offset + s.length)
    .map((s) => s.packetNumber)
    .sort((a, b) => a - b);
}

export interface ParsedSession {
  messages: Message[];
  /** Stream bytes outside of any message (lost synchronisation, garbage). */
  unframed: { direction: Direction; offset: number; length: number }[];
}

/** Frames and parses both directions of a session; messages are ordered by time. */
export function parseSession(spec: InterpretationSpec, input: SessionInput): Message[] {
  return parseSessionDetailed(spec, input).messages;
}

export function parseSessionDetailed(spec: InterpretationSpec, input: SessionInput): ParsedSession {
  const messages: Message[] = [];
  const unframedAll: ParsedSession['unframed'] = [];

  for (const dir of ['c2s', 's2c'] as const) {
    const stream = input.streams[dir];
    const unknownMask = buildMask(stream.bytes.length, stream.gaps);
    const { frames, unframed } = frameStream(spec, stream);
    unframedAll.push(...unframed.map((u) => ({ direction: dir, ...u })));

    for (const frame of frames) {
      const bytes = stream.bytes.subarray(frame.offset, frame.offset + frame.length);
      const unknown = unknownMask.subarray(frame.offset, frame.offset + frame.length);
      const rules = matchRules(spec, bytes, dir);
      const rule = rules[0];
      const covered = new Uint8Array(bytes.length);
      const fields = rule ? parseFields(rule.fields, { msg: bytes, unknown, covered }, 0, '') : [];

      const message: Message = {
        sessionId: input.session.id,
        direction: dir,
        index: 0,
        offset: frame.offset,
        length: frame.length,
        timestamp: timestampAt(stream.segments, frame.offset).ts,
        packetNumbers: packetsFor(stream.segments, frame.offset, frame.length),
        ruleId: rule?.id ?? null,
        ruleTitle: rule?.title,
        candidateRuleIds: rules.length > 1 ? rules.map((r) => r.id) : undefined,
        status: 'ok',
        issues: [...frame.issues],
        fields,
        unknown: uncovered(covered),
        data: bytesToBase64(bytes),
      };

      if (rule) applyAssertions(spec, message, rule, input.actions);

      const hasViolation = flatten(message.fields).some((f) => f.status === 'violation');
      let status: MessageStatus = 'ok';
      if (!rule) status = 'unknown_type';
      else if (frame.truncated) status = 'partial';
      else if (rules.length > 1) status = 'ambiguous';
      else if (hasViolation) status = 'violation';
      message.status = status;
      if (rules.length > 1) message.issues.push(`Подходят правила: ${rules.map((r) => r.id).join(', ')}`);
      messages.push(message);
    }
  }

  messages.sort((a, b) => a.timestamp.localeCompare(b.timestamp) || a.offset - b.offset);
  messages.forEach((m, i) => (m.index = i));
  return { messages, unframed: unframedAll };
}

export function flatten(fields: ParsedField[]): ParsedField[] {
  return fields.flatMap((f) => [f, ...flatten(f.children ?? [])]);
}

/* -------------------------------------------------------------------------- */
/*  Validation                                                                */
/* -------------------------------------------------------------------------- */

const FIELD_TYPES = new Set(['u8', 'i8', 'u16', 'i16', 'u32', 'i32', 'bytes', 'ascii', 'array']);

export function validateSpec(input: unknown): ValidationResult {
  const issues: SpecIssue[] = [];
  const err = (path: string, message: string) => issues.push({ path, message, severity: 'error' });
  const warn = (path: string, message: string) => issues.push({ path, message, severity: 'warning' });

  if (typeof input !== 'object' || input === null) {
    err('$', 'Ожидается JSON-объект');
    return { valid: false, issues };
  }
  const spec = input as Partial<InterpretationSpec>;
  if (spec.format !== 'protolab/v1') err('$.format', 'Поддерживается только формат "protolab/v1"');
  if (!spec.protocol) warn('$.protocol', 'Не указано имя протокола');

  const framing = spec.framing as InterpretationSpec['framing'] | undefined;
  if (!framing) err('$.framing', 'Не задан способ выделения сообщений');
  else if (framing.kind === 'length_prefixed') {
    if (![1, 2, 4].includes(framing.lengthField?.size)) err('$.framing.lengthField.size', 'Размер поля длины: 1, 2 или 4');
    if (typeof framing.lengthAdjust !== 'number') err('$.framing.lengthAdjust', 'Ожидается число');
    if (framing.sync && !parseHex(framing.sync)) err('$.framing.sync', 'Некорректный hex');
    if (!framing.sync) warn('$.framing.sync', 'Без sync невозможна ресинхронизация после пропуска');
  } else if (framing.kind === 'delimiter') {
    if (!parseHex(framing.delimiter ?? '')) err('$.framing.delimiter', 'Некорректный hex');
  } else if (framing.kind !== 'fixed') {
    err('$.framing.kind', 'Ожидается length_prefixed, delimiter или fixed');
  }

  const ids = new Set<string>();
  (spec.messages ?? []).forEach((rule, i) => {
    const p = `$.messages[${i}]`;
    if (!rule.id) err(`${p}.id`, 'Не задан id правила');
    else if (ids.has(rule.id)) err(`${p}.id`, `Повторяющийся id "${rule.id}"`);
    ids.add(rule.id);
    if (!rule.match?.length) warn(`${p}.match`, 'Правило без условий совпадёт с любым сообщением');
    rule.match?.forEach((c, j) => !parseHex(c.hex ?? '') && err(`${p}.match[${j}].hex`, 'Некорректный hex'));
    validateFields(rule.fields ?? [], `${p}.fields`, err);
  });
  if (!spec.messages?.length) warn('$.messages', 'Нет ни одного правила сообщения');

  (spec.assertions ?? []).forEach((a, i) => {
    if (!ids.has(a.rule)) err(`$.assertions[${i}].rule`, `Нет правила "${a.rule}"`);
    a.checks?.forEach((c, j) => {
      if (!/^(params|observed)\.\w+$/.test(c.equals ?? '')) err(`$.assertions[${i}].checks[${j}].equals`, 'Ожидается "params.<имя>" или "observed.<имя>"');
    });
  });

  return { valid: !issues.some((i) => i.severity === 'error'), issues };
}

function validateFields(fields: FieldRule[], path: string, err: (p: string, m: string) => void): void {
  const names = new Set<string>();
  fields.forEach((f, i) => {
    const p = `${path}[${i}]`;
    if (!f.name) err(`${p}.name`, 'Не задано имя поля');
    if (names.has(f.name)) err(`${p}.name`, `Повторяющееся имя "${f.name}"`);
    names.add(f.name);
    if (!FIELD_TYPES.has(f.type)) err(`${p}.type`, `Неизвестный тип "${f.type}"`);
    if (typeof f.offset !== 'number') err(`${p}.offset`, 'Ожидается число');
    if (f.type === 'array') {
      if (!f.count || !names.has(f.count)) err(`${p}.count`, 'count должен ссылаться на предыдущее поле');
      if (!f.itemSize) err(`${p}.itemSize`, 'Не задан размер элемента');
      validateFields(f.items ?? [], `${p}.items`, err);
    }
    if ((f.type === 'bytes' || f.type === 'ascii') && f.length === undefined) err(`${p}.length`, 'Не задана длина');
  });
}
