/**
 * ThermoLink — a synthetic binary protocol of a thermostat controller used as
 * demo data. Frame layout (the "truth" the researcher has to discover):
 *
 *   0      1     2    3..4          5..5+n    last
 *   A5 | type | seq | len (u16 LE) | payload | sum8(bytes[0..last))
 *
 * Responses use `type | 0x80`.
 */
export const PORT = 5020;

export const TYPE = {
  READ_PARAMS: 0x01,
  SET_PARAM: 0x02,
  GET_MEASUREMENT: 0x03,
  READ_LOG: 0x04,
  HEARTBEAT: 0x10,
} as const;

export const PARAM = {
  setpoint: 1,
  hysteresis: 2,
  mode: 3,
  fan: 4,
  interval: 5,
  calibration: 6,
} as const;

export type ParamName = keyof typeof PARAM;

const u16 = (v: number) => [v & 0xff, (v >> 8) & 0xff];
const u32 = (v: number) => [v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff];

export function frame(type: number, seq: number, payload: number[]): Uint8Array {
  const head = [0xa5, type, seq & 0xff, ...u16(payload.length), ...payload];
  const sum = head.reduce((acc, b) => (acc + b) & 0xff, 0);
  return Uint8Array.from([...head, sum]);
}

export function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let pos = 0;
  for (const p of parts) {
    out.set(p, pos);
    pos += p.length;
  }
  return out;
}

export interface LogEntry {
  ts: number;
  code: number;
  value: number;
  flags: number;
}

export const msg = {
  readParamsReq: (seq: number) => frame(TYPE.READ_PARAMS, seq, []),
  readParamsResp: (seq: number, params: Partial<Record<ParamName, number>>) => {
    const entries = Object.entries(params) as [ParamName, number][];
    return frame(TYPE.READ_PARAMS | 0x80, seq, [entries.length, ...entries.flatMap(([k, v]) => [PARAM[k], ...u16(v)])]);
  },
  setParamReq: (seq: number, param: ParamName, value: number) => frame(TYPE.SET_PARAM, seq, [PARAM[param], ...u16(value)]),
  setParamResp: (seq: number, param: ParamName, status = 0) => frame(TYPE.SET_PARAM | 0x80, seq, [status, PARAM[param]]),
  getMeasurementReq: (seq: number, channel: number) => frame(TYPE.GET_MEASUREMENT, seq, [channel]),
  getMeasurementResp: (seq: number, channel: number, temperature: number, humidity: number, uptime: number) =>
    frame(TYPE.GET_MEASUREMENT | 0x80, seq, [
      channel,
      ...u16(Math.round(temperature * 10)),
      ...u16(Math.round(humidity * 10)),
      ...u32(uptime),
    ]),
  readLogReq: (seq: number, count: number) => frame(TYPE.READ_LOG, seq, [count]),
  readLogResp: (seq: number, entries: LogEntry[]) =>
    frame(TYPE.READ_LOG | 0x80, seq, [
      entries.length,
      ...entries.flatMap((e) => [...u32(e.ts), e.code, ...u16(e.value), e.flags]),
    ]),
  heartbeat: (seq: number) => frame(TYPE.HEARTBEAT, seq, []),
  heartbeatAck: (seq: number) => frame(TYPE.HEARTBEAT | 0x80, seq, []),
};

/** Deterministic PRNG (mulberry32) so demo data is identical on every load. */
export function prng(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export function logEntries(count: number, seed: number): LogEntry[] {
  const rnd = prng(seed);
  let ts = 1_726_000_000;
  return Array.from({ length: count }, () => {
    ts += 300 + Math.floor(rnd() * 600);
    return {
      ts,
      code: [1, 2, 3, 7][Math.floor(rnd() * 4)],
      value: Math.round((15 + rnd() * 12) * 10),
      flags: rnd() > 0.8 ? 1 : 0,
    };
  });
}
