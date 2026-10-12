import type { Hypothesis } from '@/entities/hypothesis';
import type { FieldRule, InterpretationSpec, MessageRule } from '@/entities/interpretation';
import type { MeaningStatus } from '@/entities/message';
import { PORT } from './thermolink';

/* -------------------------------------------------------------------------- */
/*  Interpretation versions — the research history of "ThermoLink"             */
/* -------------------------------------------------------------------------- */

const header = (seqStatus: MeaningStatus): FieldRule[] => [
  { name: 'magic', offset: 0, type: 'u8', expect: { equals: 0xa5 }, meaning: 'Маркер начала кадра', status: 'confirmed', hypothesis: 'h-01' },
  { name: 'type', offset: 1, type: 'u8', meaning: 'Код типа сообщения', status: 'structural' },
  { name: 'seq', offset: 2, type: 'u8', meaning: 'Номер запроса (ответ повторяет номер)', status: seqStatus, hypothesis: 'h-07' },
  { name: 'length', offset: 3, type: 'u16', endian: 'le', meaning: 'Длина полезной нагрузки, Б', status: 'confirmed', hypothesis: 'h-02' },
];

const checksum: FieldRule = {
  name: 'checksum',
  offset: -1,
  type: 'u8',
  checksum: 'sum8',
  meaning: 'Контрольная сумма sum8 всех предыдущих байтов',
  status: 'hypothesis',
  hypothesis: 'h-05',
};

const match = (type: string): MessageRule['match'] => [
  { offset: 0, hex: 'A5' },
  { offset: 1, hex: type },
];

const PARAM_ENUM = {
  '1': 'уставка',
  '2': 'гистерезис',
  '3': 'режим',
  '4': 'вентилятор',
  '5': 'интервал',
  '6': 'калибровка',
};

const FRAMING: InterpretationSpec['framing'] = {
  kind: 'length_prefixed',
  lengthField: { offset: 3, size: 2, endian: 'le' },
  lengthAdjust: 6,
  minLength: 6,
  maxLength: 4096,
  sync: 'A5',
};

export const SPEC_V1: InterpretationSpec = {
  format: 'protolab/v1',
  protocol: 'ThermoLink',
  transport: { port: PORT },
  framing: FRAMING,
  messages: [
    { id: 'read_params_req', title: 'Чтение параметров — запрос', direction: 'c2s', match: match('01'), fields: header('hypothesis') },
    {
      id: 'read_params_resp',
      title: 'Чтение параметров — ответ',
      direction: 's2c',
      match: match('81'),
      fields: [
        ...header('hypothesis'),
        { name: 'count', offset: 5, type: 'u8', meaning: 'Количество параметров', status: 'hypothesis' },
        {
          name: 'entries',
          offset: 6,
          type: 'array',
          count: 'count',
          itemSize: 3,
          items: [
            { name: 'param_id', offset: 0, type: 'u8', meaning: 'Идентификатор параметра', status: 'hypothesis' },
            { name: 'value', offset: 1, type: 'u16', endian: 'le', meaning: 'Значение параметра', status: 'hypothesis' },
          ],
        },
      ],
    },
    {
      id: 'set_param_req',
      title: 'Изменение настройки — запрос',
      direction: 'c2s',
      match: match('02'),
      hypothesis: 'h-03',
      fields: [
        ...header('hypothesis'),
        { name: 'param_id', offset: 5, type: 'u8', meaning: 'Идентификатор параметра', status: 'hypothesis' },
        { name: 'value', offset: 6, type: 'u8', meaning: 'Новое значение', status: 'hypothesis', hypothesis: 'h-03' },
      ],
    },
    {
      id: 'set_param_resp',
      title: 'Изменение настройки — ответ',
      direction: 's2c',
      match: match('82'),
      fields: [
        ...header('hypothesis'),
        { name: 'status', offset: 5, type: 'u8', enum: { '0': 'OK', '1': 'вне диапазона' }, meaning: 'Результат', status: 'hypothesis' },
        { name: 'param_id', offset: 6, type: 'u8', meaning: 'Идентификатор параметра', status: 'hypothesis' },
      ],
    },
    {
      id: 'get_measurement_req',
      title: 'Измерение — запрос',
      direction: 'c2s',
      match: match('03'),
      fields: [...header('hypothesis'), { name: 'channel', offset: 5, type: 'u8', meaning: 'Номер канала', status: 'hypothesis' }],
    },
    {
      id: 'get_measurement_resp',
      title: 'Измерение — ответ',
      direction: 's2c',
      match: match('83'),
      fields: [
        ...header('hypothesis'),
        { name: 'channel', offset: 5, type: 'u8', meaning: 'Номер канала', status: 'hypothesis' },
        { name: 'temperature', offset: 6, type: 'u16', endian: 'le', scale: 0.1, unit: '°C', meaning: 'Температура', status: 'hypothesis', hypothesis: 'h-06' },
        { name: 'humidity', offset: 8, type: 'u16', endian: 'le', scale: 0.1, unit: '%', meaning: 'Влажность', status: 'hypothesis' },
      ],
    },
  ],
  assertions: [
    {
      id: 'a-set-value',
      rule: 'set_param_req',
      actionKind: 'set_param',
      checks: [
        { field: 'param_id', equals: 'params.paramId' },
        { field: 'value', equals: 'params.value' },
      ],
      hypothesis: 'h-03',
    },
    {
      id: 'a-measurement',
      rule: 'get_measurement_resp',
      actionKind: 'get_measurement',
      checks: [
        { field: 'temperature', equals: 'observed.temperature' },
        { field: 'humidity', equals: 'observed.humidity' },
      ],
      hypothesis: 'h-06',
    },
  ],
};

function withV2Fields(seqStatus: MeaningStatus, paramEnum: boolean): InterpretationSpec['messages'] {
  const h = header(seqStatus);
  const paramId: FieldRule = {
    name: 'param_id',
    offset: 5,
    type: 'u8',
    meaning: 'Идентификатор параметра',
    status: paramEnum ? 'confirmed' : 'hypothesis',
    ...(paramEnum ? { enum: PARAM_ENUM } : {}),
  };
  return [
    { id: 'read_params_req', title: 'Чтение параметров — запрос', direction: 'c2s', match: match('01'), fields: [...h, checksum] },
    {
      id: 'read_params_resp',
      title: 'Чтение параметров — ответ',
      direction: 's2c',
      match: match('81'),
      fields: [
        ...h,
        { name: 'count', offset: 5, type: 'u8', meaning: 'Количество параметров', status: 'confirmed' },
        {
          name: 'entries',
          offset: 6,
          type: 'array',
          count: 'count',
          itemSize: 3,
          items: [
            { ...paramId, offset: 0 },
            { name: 'value', offset: 1, type: 'i16', endian: 'le', meaning: 'Значение параметра', status: 'confirmed', hypothesis: 'h-04' },
          ],
        },
        checksum,
      ],
    },
    {
      id: 'set_param_req',
      title: 'Изменение настройки — запрос',
      direction: 'c2s',
      match: match('02'),
      hypothesis: 'h-04',
      fields: [
        ...h,
        paramId,
        { name: 'value', offset: 6, type: 'i16', endian: 'le', meaning: 'Новое значение', status: 'confirmed', hypothesis: 'h-04' },
        checksum,
      ],
    },
    {
      id: 'set_param_resp',
      title: 'Изменение настройки — ответ',
      direction: 's2c',
      match: match('82'),
      fields: [
        ...h,
        {
          name: 'status',
          offset: 5,
          type: 'u8',
          enum: { '0': 'OK', '1': 'вне диапазона' },
          meaning: 'Результат',
          status: 'hypothesis',
          ...(paramEnum ? { expect: { oneOf: [0, 1] } } : {}),
        },
        { ...paramId, offset: 6 },
        checksum,
      ],
    },
    {
      id: 'get_measurement_req',
      title: 'Измерение — запрос',
      direction: 'c2s',
      match: match('03'),
      fields: [...h, { name: 'channel', offset: 5, type: 'u8', meaning: 'Номер канала', status: 'confirmed' }, checksum],
    },
    {
      id: 'get_measurement_resp',
      title: 'Измерение — ответ',
      direction: 's2c',
      match: match('83'),
      fields: [
        ...h,
        { name: 'channel', offset: 5, type: 'u8', meaning: 'Номер канала', status: 'confirmed' },
        { name: 'temperature', offset: 6, type: 'i16', endian: 'le', scale: 0.1, unit: '°C', meaning: 'Температура', status: 'confirmed', hypothesis: 'h-06' },
        { name: 'humidity', offset: 8, type: 'u16', endian: 'le', scale: 0.1, unit: '%', meaning: 'Влажность', status: 'confirmed' },
        { name: 'uptime', offset: 10, type: 'u32', endian: 'le', unit: 'с', meaning: 'Время работы устройства?', status: 'hypothesis', hypothesis: 'h-08' },
        checksum,
      ],
    },
    {
      id: 'read_log_req',
      title: 'Чтение журнала — запрос',
      direction: 'c2s',
      match: match('04'),
      fields: [...h, { name: 'count', offset: 5, type: 'u8', meaning: 'Запрошено записей', status: 'hypothesis' }, checksum],
    },
    {
      id: 'read_log_resp',
      title: 'Чтение журнала — ответ',
      direction: 's2c',
      match: match('84'),
      fields: [
        ...h,
        { name: 'count', offset: 5, type: 'u8', meaning: 'Количество записей', status: 'confirmed' },
        {
          name: 'entries',
          offset: 6,
          type: 'array',
          count: 'count',
          itemSize: 8,
          items: [
            { name: 'timestamp', offset: 0, type: 'u32', endian: 'le', meaning: 'Unix-время события?', status: 'hypothesis' },
            { name: 'code', offset: 4, type: 'u8', meaning: 'Код события', status: 'hypothesis' },
            { name: 'value', offset: 5, type: 'i16', endian: 'le', scale: 0.1, meaning: 'Значение', status: 'hypothesis' },
            { name: 'flags', offset: 7, type: 'u8', meaning: 'Флаги (назначение неизвестно)', status: 'structural' },
          ],
        },
        checksum,
      ],
    },
    { id: 'heartbeat', title: 'Heartbeat', direction: 'c2s', match: match('10'), fields: [...h, checksum] },
    { id: 'heartbeat_ack', title: 'Heartbeat — ответ', direction: 's2c', match: match('90'), fields: [...h, checksum] },
  ];
}

export const SPEC_V2: InterpretationSpec = {
  ...SPEC_V1,
  messages: withV2Fields('hypothesis', false),
  assertions: SPEC_V1.assertions!.map((a) => (a.id === 'a-set-value' ? { ...a, hypothesis: 'h-04' } : a)),
};

export const SPEC_V3: InterpretationSpec = {
  ...SPEC_V2,
  messages: withV2Fields('confirmed', true),
};

/* -------------------------------------------------------------------------- */
/*  Hypotheses                                                                 */
/* -------------------------------------------------------------------------- */

type HypothesisSeed = Omit<Hypothesis, 'projectId' | 'observationIds' | 'counterexamples'>;

export const HYPOTHESIS_SEEDS: HypothesisSeed[] = [
  {
    id: 'h-01',
    title: 'Байт 0 = 0xA5 — маркер начала кадра',
    statement: 'Каждое сообщение в обоих направлениях начинается с байта 0xA5.',
    status: 'supported',
    alternatives: ['Часть поля версии протокола'],
    ruleRefs: ['*.magic'],
    createdAt: '2026-09-14T11:20:00Z',
    updatedAt: '2026-09-15T14:05:00Z',
    history: [
      { at: '2026-09-14T11:20:00Z', status: 'proposed', note: 'Замечено в первых 8 сессиях' },
      { at: '2026-09-15T14:05:00Z', status: 'supported', note: 'Выполняется на расширенном корпусе (run-3)' },
    ],
  },
  {
    id: 'h-02',
    title: 'Байты 3–4 — длина полезной нагрузки (u16 LE)',
    statement: 'Полная длина сообщения = значение u16 LE по смещению 3 + 6. Граница TCP-сегмента не совпадает с границей сообщения.',
    status: 'supported',
    alternatives: ['Длина включает заголовок (+1/+5)'],
    ruleRefs: ['framing.lengthField'],
    createdAt: '2026-09-14T11:40:00Z',
    updatedAt: '2026-09-14T13:10:00Z',
    history: [
      { at: '2026-09-14T11:40:00Z', status: 'proposed', note: 'Ответы разного размера коррелируют с байтом 3' },
      { at: '2026-09-14T13:10:00Z', status: 'supported', note: 'Ответ чтения журнала (167 Б) в 3 сегментах кадрируется корректно' },
    ],
  },
  {
    id: 'h-03',
    title: 'Значение настройки — u8 по смещению 6',
    statement: 'В запросе 0x02 новое значение настройки кодируется одним байтом по смещению 6.',
    status: 'refuted',
    alternatives: ['u16/i16 LE по смещению 6–7'],
    ruleRefs: ['set_param_req.value (v1)'],
    createdAt: '2026-09-14T12:00:00Z',
    updatedAt: '2026-09-15T10:40:00Z',
    history: [
      { at: '2026-09-14T12:00:00Z', status: 'proposed', note: '«Задать 21» → 0x15 по смещению 6' },
      { at: '2026-09-14T12:30:00Z', status: 'testing', note: 'Совпало для 21, 23, 18 — но байт 7 всегда 0x00' },
      { at: '2026-09-15T10:40:00Z', status: 'refuted', note: 'Интервал 600 → 58 02, калибровка −15 → F1 FF (run-2)' },
    ],
  },
  {
    id: 'h-04',
    title: 'Значение настройки — i16 LE по смещению 6–7',
    statement: 'Новое значение настройки — знаковое 16-битное целое little-endian; масштаб зависит от параметра.',
    status: 'supported',
    alternatives: ['u16 LE (опровергается калибровкой −15)'],
    ruleRefs: ['set_param_req.value', 'read_params_resp.entries[].value'],
    createdAt: '2026-09-15T10:45:00Z',
    updatedAt: '2026-09-15T14:05:00Z',
    history: [
      { at: '2026-09-15T10:45:00Z', status: 'testing', note: 'Объясняет 21/23/18/2/600/−15' },
      { at: '2026-09-15T14:05:00Z', status: 'supported', note: 'Проверка a-set-value проходит на всём расширенном корпусе' },
    ],
  },
  {
    id: 'h-05',
    title: 'Последний байт — контрольная сумма sum8',
    statement: 'Последний байт = сумма всех предыдущих байтов сообщения по модулю 256.',
    status: 'testing',
    alternatives: ['XOR всех байтов', 'CRC-8'],
    ruleRefs: ['*.checksum'],
    createdAt: '2026-09-15T13:00:00Z',
    updatedAt: '2026-09-15T14:05:00Z',
    history: [{ at: '2026-09-15T13:00:00Z', status: 'testing', note: 'Совпадает везде, но нет примера с намеренно испорченным кадром' }],
  },
  {
    id: 'h-06',
    title: 'Температура — i16 LE × 0.1 °C',
    statement: 'В ответе 0x83 температура — знаковое i16 LE по смещению 6, масштаб 0.1 °C.',
    status: 'supported',
    alternatives: ['u16 × 0.1 (опровергается −5.5 °C)'],
    ruleRefs: ['get_measurement_resp.temperature'],
    createdAt: '2026-09-14T12:40:00Z',
    updatedAt: '2026-09-15T14:05:00Z',
    history: [
      { at: '2026-09-14T12:40:00Z', status: 'testing', note: '22.4 → E0 00, 19.8 → C6 00' },
      { at: '2026-09-15T14:05:00Z', status: 'supported', note: 'Канал 3: −5.5 °C → C9 FF — работает только знаковое' },
    ],
  },
  {
    id: 'h-07',
    title: 'Байт 2 — номер запроса',
    statement: 'Клиент увеличивает байт 2 в каждом запросе сессии; ответ повторяет значение запроса.',
    status: 'supported',
    alternatives: ['Номер канала/устройства'],
    ruleRefs: ['*.seq'],
    createdAt: '2026-09-14T13:30:00Z',
    updatedAt: '2026-09-16T10:00:00Z',
    history: [
      { at: '2026-09-14T13:30:00Z', status: 'proposed', note: 'В сессии с двумя запросами значения 1 и 2' },
      { at: '2026-09-16T10:00:00Z', status: 'supported', note: 'Подтверждено на всех многозапросных сессиях' },
    ],
  },
  {
    id: 'h-08',
    title: 'u32 по смещению 10 в ответе 0x83 — время работы',
    statement: 'Монотонно растущее значение; разница между измерениями ≈ разнице времени захвата в секундах.',
    status: 'proposed',
    alternatives: ['Счётчик измерений', 'Unix-время'],
    ruleRefs: ['get_measurement_resp.uptime'],
    createdAt: '2026-09-15T13:30:00Z',
    updatedAt: '2026-09-15T13:30:00Z',
    history: [{ at: '2026-09-15T13:30:00Z', status: 'proposed', note: '86412 → 86473 за 60 с захвата' }],
  },
];
