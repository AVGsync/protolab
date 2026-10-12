import type { ActionEntry } from '@/entities/action';
import type { CaptureSpec, SessionSpec } from './generator';
import { concat, logEntries, msg, PORT } from './thermolink';
import { MS, SECOND } from './time';

export const PROJECT_ID = 'prj-tc200';
export const EMPTY_PROJECT_ID = 'prj-bench';

const DEVICE = { ip: '192.168.10.50', port: PORT };
const client = (port: number) => ({ ip: '192.168.10.12', port });

type ActionSeed = Omit<ActionEntry, 'projectId' | 'links' | 'source'> & { sessionKey: string };

/* -------------------------------------------------------------------------- */
/*  Capture 1 — main research corpus                                           */
/* -------------------------------------------------------------------------- */

const DAY1 = '2026-09-14T10:00:00Z';

const day1Sessions: SessionSpec[] = [
  {
    key: 'read-params',
    client: client(51001),
    server: DEVICE,
    startUs: 5 * SECOND,
    actionId: 'act-01',
    exchanges: [
      { req: msg.readParamsReq(1), resp: msg.readParamsResp(1, { setpoint: 22, hysteresis: 1, mode: 1, fan: 2 }) },
    ],
  },
  {
    key: 'set-21',
    client: client(51002),
    server: DEVICE,
    startUs: 70 * SECOND,
    actionId: 'act-02',
    exchanges: [
      { req: msg.setParamReq(1, 'setpoint', 21), resp: msg.setParamResp(1, 'setpoint') },
      { req: msg.readParamsReq(2), resp: msg.readParamsResp(2, { setpoint: 21, hysteresis: 1, mode: 1, fan: 2 }) },
    ],
  },
  {
    key: 'set-23',
    client: client(51003),
    server: DEVICE,
    startUs: 150 * SECOND,
    actionId: 'act-03',
    exchanges: [{ req: msg.setParamReq(1, 'setpoint', 23), resp: msg.setParamResp(1, 'setpoint') }],
  },
  {
    key: 'set-18',
    client: client(51004),
    server: DEVICE,
    startUs: 220 * SECOND,
    actionId: 'act-04',
    // The request crosses a TCP segment boundary.
    exchanges: [{ req: msg.setParamReq(1, 'setpoint', 18), reqSplit: [4], resp: msg.setParamResp(1, 'setpoint') }],
  },
  {
    key: 'set-mode',
    // Same client port as "set-23": a distinct connection, never merged.
    client: client(51003),
    server: DEVICE,
    startUs: 290 * SECOND,
    actionId: 'act-05',
    exchanges: [{ req: msg.setParamReq(1, 'mode', 2), resp: msg.setParamResp(1, 'mode') }],
  },
  {
    key: 'meas-1',
    client: client(51006),
    server: DEVICE,
    startUs: 360 * SECOND,
    actionId: 'act-06',
    // Two messages in one TCP segment (heartbeat + request).
    exchanges: [
      {
        req: concat(msg.heartbeat(1), msg.getMeasurementReq(2, 1)),
        resp: concat(msg.heartbeatAck(1), msg.getMeasurementResp(2, 1, 22.4, 41.0, 86_412)),
      },
    ],
  },
  {
    key: 'meas-2',
    client: client(51007),
    server: DEVICE,
    startUs: 420 * SECOND,
    actionId: 'act-07',
    exchanges: [{ req: msg.getMeasurementReq(1, 2), resp: msg.getMeasurementResp(1, 2, 19.8, 55.5, 86_473) }],
  },
  {
    key: 'read-log',
    client: client(51008),
    server: DEVICE,
    startUs: 480 * SECOND,
    actionId: 'act-08',
    exchanges: [{ req: msg.readLogReq(1, 20), resp: msg.readLogResp(1, logEntries(20, 7)), respSplit: [64, 128] }],
    defects: {
      retransmit: { dir: 's2c', index: 1 },
      reorder: { dir: 's2c', index: 1 },
    },
  },
  {
    key: 'tls-noise',
    client: client(51544),
    server: { ip: '93.184.216.34', port: 443 },
    startUs: 200 * SECOND,
    exchanges: [
      { req: noise(517, 1), resp: noise(1380, 2), respSplit: [700] },
      { req: noise(126, 3), resp: noise(240, 4), pauseUs: 90 * MS },
    ],
  },
];

/* -------------------------------------------------------------------------- */
/*  Capture 2 — additional record with new parameter values                   */
/* -------------------------------------------------------------------------- */

const DAY2 = '2026-09-15T09:30:00Z';

const day2Sessions: SessionSpec[] = [
  {
    key: 'set-interval',
    client: client(52001),
    server: DEVICE,
    startUs: 10 * SECOND,
    actionId: 'act-09',
    exchanges: [{ req: msg.setParamReq(1, 'interval', 600), resp: msg.setParamResp(1, 'interval') }],
  },
  {
    key: 'set-calibration',
    client: client(52002),
    server: DEVICE,
    startUs: 60 * SECOND,
    actionId: 'act-10',
    exchanges: [{ req: msg.setParamReq(1, 'calibration', -15), resp: msg.setParamResp(1, 'calibration') }],
  },
  {
    key: 'meas-outdoor',
    client: client(52003),
    server: DEVICE,
    startUs: 120 * SECOND,
    actionId: 'act-11',
    exchanges: [{ req: msg.getMeasurementReq(1, 3), resp: msg.getMeasurementResp(1, 3, -5.5, 80.0, 167_020) }],
  },
  {
    key: 'set-25',
    client: client(52004),
    server: DEVICE,
    startUs: 180 * SECOND,
    actionId: 'act-12',
    exchanges: [{ req: msg.setParamReq(1, 'setpoint', 25), resp: msg.setParamResp(1, 'setpoint') }],
  },
  {
    key: 'read-params-2',
    client: client(52005),
    server: DEVICE,
    startUs: 240 * SECOND,
    actionId: 'act-13',
    exchanges: [
      {
        req: msg.readParamsReq(1),
        resp: msg.readParamsResp(1, { setpoint: 25, hysteresis: 1, mode: 2, fan: 2, interval: 600, calibration: -15 }),
      },
    ],
  },
];

/* -------------------------------------------------------------------------- */
/*  Capture 3 — capture defects (diagnostics showcase)                         */
/* -------------------------------------------------------------------------- */

const DAY3 = '2026-09-15T16:00:00Z';

const day3Sessions: SessionSpec[] = [
  {
    key: 'lost-segment',
    client: client(53001),
    server: DEVICE,
    startUs: 2 * SECOND,
    exchanges: [
      { req: msg.readLogReq(1, 12), resp: msg.readLogResp(1, logEntries(12, 11)), respSplit: [40, 80] },
      { req: msg.readParamsReq(2), resp: msg.readParamsResp(2, { setpoint: 25, hysteresis: 1 }) },
    ],
    defects: { drop: { dir: 's2c', index: 1 } },
  },
  {
    key: 'truncated',
    client: client(53002),
    server: DEVICE,
    startUs: 30 * SECOND,
    exchanges: [{ req: msg.readLogReq(1, 16), resp: msg.readLogResp(1, logEntries(16, 13)) }],
    defects: { truncate: { dir: 's2c', index: 0, keep: 42 } },
  },
  {
    key: 'conflict',
    client: client(53003),
    server: DEVICE,
    startUs: 60 * SECOND,
    exchanges: [
      { req: msg.setParamReq(1, 'setpoint', 24), resp: msg.setParamResp(1, 'setpoint') },
      { req: msg.readParamsReq(2), resp: msg.readParamsResp(2, { setpoint: 24 }) },
    ],
    defects: { retransmit: { dir: 'c2s', index: 0, conflict: true } },
  },
  {
    key: 'no-handshake',
    client: client(53004),
    server: DEVICE,
    startUs: 90 * SECOND,
    exchanges: [
      { req: msg.setParamReq(1, 'fan', 3), reqSplit: [5], resp: msg.setParamResp(1, 'fan') },
      { req: msg.getMeasurementReq(2, 1), resp: msg.getMeasurementResp(2, 1, 23.1, 39.5, 190_001) },
    ],
    // SYN, SYN-ACK, ACK and the first half of the request were not captured.
    defects: { dropLeading: 4, noClose: true },
  },
  {
    key: 'checksum',
    client: client(53005),
    server: DEVICE,
    startUs: 120 * SECOND,
    exchanges: [{ req: msg.getMeasurementReq(1, 1), resp: msg.getMeasurementResp(1, 1, 23.4, 39.0, 190_031) }],
    defects: { checksumWarnings: 'c2s' },
  },
];

function noise(length: number, seed: number): Uint8Array {
  let x = seed * 2654435761;
  const out = new Uint8Array(length);
  out[0] = 0x16;
  out[1] = 0x03;
  out[2] = 0x03;
  for (let i = 3; i < length; i++) {
    x = (x * 1103515245 + 12345) >>> 0;
    out[i] = (x >>> 16) & 0xff;
  }
  return out;
}

export const CAPTURE_SPECS: CaptureSpec[] = [
  {
    id: 'cap-1',
    projectId: PROJECT_ID,
    fileName: 'tc200_actions_2026-09-14.pcapng',
    format: 'pcapng',
    startIso: DAY1,
    importedAt: '2026-09-14T11:02:13Z',
    snapLen: 262144,
    targetPorts: [PORT],
    sessions: day1Sessions,
    nonTcpPackets: 6,
  },
  {
    id: 'cap-2',
    projectId: PROJECT_ID,
    fileName: 'tc200_extra_2026-09-15.pcapng',
    format: 'pcapng',
    startIso: DAY2,
    importedAt: '2026-09-15T10:05:40Z',
    snapLen: 262144,
    targetPorts: [PORT],
    sessions: day2Sessions,
  },
  {
    id: 'cap-3',
    projectId: PROJECT_ID,
    fileName: 'tc200_defects.pcap',
    format: 'pcap',
    startIso: DAY3,
    importedAt: '2026-09-15T16:40:02Z',
    snapLen: 96,
    targetPorts: [PORT],
    sessions: day3Sessions,
  },
];

/** Template reused when a capture is uploaded in mock mode (content is not parsed). */
export const UPLOAD_TEMPLATE = day2Sessions;

const at = (base: string, us: number) => new Date(Date.parse(base) + us / 1000).toISOString();

export const ACTION_SEEDS: ActionSeed[] = [
  { id: 'act-01', sessionKey: 'read-params', timestamp: at(DAY1, 4.6 * SECOND), kind: 'read_params', label: 'Чтение параметров', params: {}, result: 'Уставка 22 °C, гистерезис 1, режим «нагрев», вентилятор 2' },
  { id: 'act-02', sessionKey: 'set-21', timestamp: at(DAY1, 69.7 * SECOND), kind: 'set_param', label: 'Задать уставку 21 °C', params: { param: 'setpoint', paramId: 1, value: 21 }, result: 'OK, клиент перечитал параметры' },
  { id: 'act-03', sessionKey: 'set-23', timestamp: at(DAY1, 149.5 * SECOND), kind: 'set_param', label: 'Задать уставку 23 °C', params: { param: 'setpoint', paramId: 1, value: 23 }, result: 'OK' },
  { id: 'act-04', sessionKey: 'set-18', timestamp: at(DAY1, 219.8 * SECOND), kind: 'set_param', label: 'Задать уставку 18 °C', params: { param: 'setpoint', paramId: 1, value: 18 }, result: 'OK' },
  { id: 'act-05', sessionKey: 'set-mode', timestamp: at(DAY1, 289.6 * SECOND), kind: 'set_param', label: 'Режим: охлаждение', params: { param: 'mode', paramId: 3, value: 2 }, result: 'OK' },
  { id: 'act-06', sessionKey: 'meas-1', timestamp: at(DAY1, 359.7 * SECOND), kind: 'get_measurement', label: 'Измерение, канал 1', params: { channel: 1 }, observed: { temperature: 22.4, humidity: 41.0 }, result: '22.4 °C, 41.0 %' },
  { id: 'act-07', sessionKey: 'meas-2', timestamp: at(DAY1, 419.8 * SECOND), kind: 'get_measurement', label: 'Измерение, канал 2', params: { channel: 2 }, observed: { temperature: 19.8, humidity: 55.5 }, result: '19.8 °C, 55.5 %' },
  { id: 'act-08', sessionKey: 'read-log', timestamp: at(DAY1, 479.6 * SECOND), kind: 'read_log', label: 'Чтение журнала (20 записей)', params: { count: 20 }, result: 'Показано 20 записей' },
  { id: 'act-09', sessionKey: 'set-interval', timestamp: at(DAY2, 9.7 * SECOND), kind: 'set_param', label: 'Интервал отчёта 600 с', params: { param: 'interval', paramId: 5, value: 600 }, result: 'OK' },
  { id: 'act-10', sessionKey: 'set-calibration', timestamp: at(DAY2, 59.8 * SECOND), kind: 'set_param', label: 'Калибровка −1.5 °C', params: { param: 'calibration', paramId: 6, value: -15 }, result: 'OK (в клиенте −1.5 °C)' },
  { id: 'act-11', sessionKey: 'meas-outdoor', timestamp: at(DAY2, 119.6 * SECOND), kind: 'get_measurement', label: 'Измерение, канал 3 (улица)', params: { channel: 3 }, observed: { temperature: -5.5, humidity: 80.0 }, result: '−5.5 °C, 80.0 %' },
  { id: 'act-12', sessionKey: 'set-25', timestamp: at(DAY2, 179.7 * SECOND), kind: 'set_param', label: 'Задать уставку 25 °C', params: { param: 'setpoint', paramId: 1, value: 25 }, result: 'OK' },
  { id: 'act-13', sessionKey: 'read-params-2', timestamp: at(DAY2, 239.6 * SECOND), kind: 'read_params', label: 'Чтение параметров', params: {}, result: '6 параметров' },
];
