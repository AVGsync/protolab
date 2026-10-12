/**
 * In-memory state of the mock backend. Rebuilt on every page load from
 * deterministic fixtures, mutated by MSW handlers.
 */
import type { ActionEntry } from '@/entities/action';
import type { Capture } from '@/entities/capture';
import type { Corpus } from '@/entities/corpus';
import type { Hypothesis } from '@/entities/hypothesis';
import type { Interpretation, InterpretationSpec, InterpretationVersion } from '@/entities/interpretation';
import type { Observation } from '@/entities/observation';
import type { Project } from '@/entities/project';
import type { Report } from '@/entities/report';
import type { Finding, VerificationRun } from '@/entities/verification';
import type { Direction, SourceRef } from '@/shared/api';
import { buildMask, toHexMasked } from '@/shared/lib';
import { ENGINE_VERSION, parseSession, type SessionInput } from './engine/interpreter';
import { verify } from './engine/verify';
import { generateCapture, type GeneratedSession } from './fixtures/generator';
import { fakeSha256 } from './fixtures/hash';
import { HYPOTHESIS_SEEDS, SPEC_V1, SPEC_V2, SPEC_V3 } from './fixtures/research';
import { ACTION_SEEDS, CAPTURE_SPECS, EMPTY_PROJECT_ID, PROJECT_ID } from './fixtures/scenarios';

export interface StoredRun {
  run: Omit<VerificationRun, 'stale' | 'staleReasons'>;
  findings: Finding[];
}

let counter = 100;
export const nextId = (prefix: string): string => `${prefix}-${++counter}`;

export const specHash = (spec: InterpretationSpec): string => fakeSha256(JSON.stringify(spec));

export const db = {
  projects: [] as Project[],
  captures: [] as Capture[],
  sessions: new Map<string, GeneratedSession>(),
  actions: [] as ActionEntry[],
  interpretations: [] as Interpretation[],
  versions: [] as InterpretationVersion[],
  hypotheses: [] as Hypothesis[],
  observations: [] as Observation[],
  corpora: [] as Corpus[],
  runs: [] as StoredRun[],
  reports: [] as Report[],
};

/* -------------------------------------------------------------------------- */
/*  Queries                                                                   */
/* -------------------------------------------------------------------------- */

export function sessionsOf(projectId: string): GeneratedSession[] {
  return [...db.sessions.values()].filter((s) => s.session.projectId === projectId);
}

export function sessionInput(sessionId: string): SessionInput | undefined {
  const s = db.sessions.get(sessionId);
  if (!s) return undefined;
  return {
    session: s.session,
    streams: {
      c2s: { bytes: s.streams.c2s.bytes, gaps: s.streams.c2s.meta.gaps, segments: s.streams.c2s.meta.segments },
      s2c: { bytes: s.streams.s2c.bytes, gaps: s.streams.s2c.meta.gaps, segments: s.streams.s2c.meta.segments },
    },
    actions: db.actions.filter((a) => a.links.some((l) => l.sessionId === sessionId)),
  };
}

export function projectWithStats(p: Project): Project {
  const by = <T extends { projectId: string }>(list: T[]) => list.filter((x) => x.projectId === p.id).length;
  return {
    ...p,
    stats: {
      captures: by(db.captures),
      sessions: sessionsOf(p.id).length,
      actions: by(db.actions),
      observations: by(db.observations),
      hypotheses: by(db.hypotheses),
      interpretations: by(db.interpretations),
      runs: db.runs.filter((r) => r.run.projectId === p.id).length,
    },
  };
}

export function runDto(stored: StoredRun): VerificationRun {
  const run = stored.run;
  const reasons: string[] = [];
  const interp = db.interpretations.find((i) => i.id === run.interpretationId);
  if (interp && interp.latestVersion > run.version) {
    reasons.push(`Проверка выполнена на v${run.version}, актуальная версия интерпретации — v${interp.latestVersion}`);
  }
  const corpus = db.corpora.find((c) => c.id === run.corpusId);
  if (corpus && corpus.hash !== run.corpusHash) reasons.push('Состав корпуса изменился после проверки');
  const project = db.projects.find((p) => p.id === run.projectId);
  if (
    project &&
    (project.settings.overlapPolicy !== run.settings.overlapPolicy ||
      project.settings.checksumPolicy !== run.settings.checksumPolicy)
  ) {
    reasons.push('Изменены настройки восстановления потоков проекта');
  }
  return { ...run, stale: run.status === 'succeeded' && reasons.length > 0, staleReasons: reasons };
}

/** Executes a verification run synchronously and stores its results. */
export function executeRun(stored: StoredRun): void {
  const version = db.versions.find((v) => v.id === stored.run.versionId)!;
  const corpus = db.corpora.find((c) => c.id === stored.run.corpusId)!;
  const inputs = corpus.sessionIds.map(sessionInput).filter((x): x is SessionInput => x !== undefined);
  const t0 = performance.now();
  const { summary, findings } = verify(version.spec, inputs);
  stored.run.summary = summary;
  stored.run.durationMs = Math.max(1, Math.round(performance.now() - t0));
  stored.run.status = 'succeeded';
  stored.findings = findings.map((f, i) => ({ ...f, id: `${stored.run.id}-f${i + 1}`, runId: stored.run.id }));
  const previous = db.runs
    .filter(
      (r) =>
        r !== stored &&
        r.run.interpretationId === stored.run.interpretationId &&
        r.run.corpusId === stored.run.corpusId &&
        r.run.startedAt < stored.run.startedAt &&
        r.run.status === 'succeeded',
    )
    .sort((a, b) => b.run.startedAt.localeCompare(a.run.startedAt))[0];
  stored.run.previousRunId = previous?.run.id;
}

export function createRun(projectId: string, versionId: string, corpusId: string, startedAt: string, id = nextId('run')): StoredRun {
  const version = db.versions.find((v) => v.id === versionId)!;
  const interp = db.interpretations.find((i) => i.id === version.interpretationId)!;
  const corpus = db.corpora.find((c) => c.id === corpusId)!;
  const project = db.projects.find((p) => p.id === projectId)!;
  const stored: StoredRun = {
    run: {
      id,
      projectId,
      interpretationId: interp.id,
      interpretationName: interp.name,
      versionId,
      version: version.version,
      versionHash: version.hash,
      corpusId,
      corpusName: corpus.name,
      corpusHash: corpus.hash,
      status: 'queued',
      startedAt,
      settings: { overlapPolicy: project.settings.overlapPolicy, checksumPolicy: project.settings.checksumPolicy },
      engineVersion: ENGINE_VERSION,
    },
    findings: [],
  };
  db.runs.push(stored);
  return stored;
}

export function corpusHash(sessionIds: string[]): string {
  const parts = [...sessionIds].sort().map((id) => `${id}:${db.sessions.get(id)?.session.capture.sha256 ?? ''}`);
  return fakeSha256(parts.join('|'));
}

export function sourceSnapshot(source: SourceRef): string {
  const s = db.sessions.get(source.sessionId);
  if (!s) return '';
  const stream = s.streams[source.direction];
  const mask = buildMask(stream.bytes.length, stream.meta.gaps);
  const end = source.offset + source.length;
  return toHexMasked(stream.bytes.subarray(source.offset, end), mask.subarray(source.offset, end));
}

export function packetsForRange(sessionId: string, dir: Direction, offset: number, length: number): number[] {
  const s = db.sessions.get(sessionId);
  if (!s) return [];
  return s.streams[dir].meta.segments
    .filter((seg) => seg.offset < offset + length && offset < seg.offset + seg.length)
    .map((seg) => seg.packetNumber);
}

/* -------------------------------------------------------------------------- */
/*  Seed                                                                      */
/* -------------------------------------------------------------------------- */

function seed(): void {
  db.projects.push(
    {
      id: PROJECT_ID,
      name: 'Термоконтроллер TC-200',
      description: 'Обмен штатного клиента TC-Config с контроллером по TCP/5020: чтение параметров, изменение настроек, измерения.',
      createdAt: '2026-09-14T10:55:00Z',
      updatedAt: '2026-09-16T10:05:00Z',
      settings: { overlapPolicy: 'first', checksumPolicy: 'warn', targetPorts: [5020] },
      stats: { captures: 0, sessions: 0, actions: 0, observations: 0, hypotheses: 0, interpretations: 0, runs: 0 },
    },
    {
      id: EMPTY_PROJECT_ID,
      name: 'Стенд HVAC-шлюза',
      description: 'Новый стенд, записи ещё не загружены.',
      createdAt: '2026-10-01T08:00:00Z',
      updatedAt: '2026-10-01T08:00:00Z',
      settings: { overlapPolicy: 'first', checksumPolicy: 'warn', targetPorts: [502] },
      stats: { captures: 0, sessions: 0, actions: 0, observations: 0, hypotheses: 0, interpretations: 0, runs: 0 },
    },
  );

  const byKey = new Map<string, string>();
  CAPTURE_SPECS.forEach((spec, ci) => {
    const generated = generateCapture(spec, (key, i) => {
      const id = `ses-${ci + 1}-${String(i + 1).padStart(2, '0')}`;
      byKey.set(key, id);
      return id;
    });
    db.captures.push(generated.capture);
    for (const s of generated.sessions) db.sessions.set(s.session.id, s);
  });

  for (const seedAction of ACTION_SEEDS) {
    const { sessionKey, ...rest } = seedAction;
    const sessionId = byKey.get(sessionKey)!;
    const session = db.sessions.get(sessionId)!.session;
    db.actions.push({
      ...rest,
      projectId: PROJECT_ID,
      source: 'import',
      links: [
        {
          sessionId,
          method: 'auto_time',
          deltaMs: Date.parse(session.startedAt) - Date.parse(rest.timestamp),
        },
      ],
    });
  }

  db.interpretations.push({
    id: 'int-thermolink',
    projectId: PROJECT_ID,
    name: 'ThermoLink',
    description: 'Бинарный протокол TC-Config ↔ TC-200: кадры A5, длина u16 LE, контрольная сумма.',
    latestVersion: 3,
    latestVersionId: 'ver-3',
    createdAt: '2026-09-14T12:50:00Z',
    updatedAt: '2026-09-16T09:55:00Z',
  });
  const versions: [string, number, InterpretationSpec, string, string][] = [
    ['ver-1', 1, SPEC_V1, '2026-09-14T12:50:00Z', 'Кадрирование по длине; запросы чтения/записи/измерения'],
    ['ver-2', 2, SPEC_V2, '2026-09-15T13:50:00Z', 'value → i16 LE, температура → i16; журнал, heartbeat, контрольная сумма'],
    ['ver-3', 3, SPEC_V3, '2026-09-16T09:55:00Z', 'seq подтверждён; перечисление параметров; ожидание статуса ответа'],
  ];
  versions.forEach(([id, version, spec, createdAt, comment], i) =>
    db.versions.push({
      id,
      interpretationId: 'int-thermolink',
      version,
      hash: specHash(spec),
      comment,
      createdAt,
      parentVersionId: i > 0 ? versions[i - 1][0] : undefined,
      spec,
    }),
  );

  const target = (captureId: string) =>
    sessionsOf(PROJECT_ID)
      .filter((s) => s.session.captureId === captureId && s.session.isTarget)
      .map((s) => s.session.id);
  const corpus = (id: string, name: string, description: string, sessionIds: string[], filter: Corpus['filter'], createdAt: string) =>
    db.corpora.push({ id, projectId: PROJECT_ID, name, description, sessionIds, filter, hash: corpusHash(sessionIds), createdAt });
  corpus('cor-main', 'Основной (14.09)', 'Все действия первого дня, целевой порт', target('cap-1'), { captureIds: ['cap-1'], onlyTarget: true }, '2026-09-14T12:55:00Z');
  corpus(
    'cor-ext',
    'Расширенный (14.09 + 15.09)',
    'Основной корпус + дополнительная запись с новыми значениями параметров',
    [...target('cap-1'), ...target('cap-2')],
    { captureIds: ['cap-1', 'cap-2'], onlyTarget: true },
    '2026-09-15T10:20:00Z',
  );
  corpus('cor-def', 'Дефекты захвата', 'Записи с потерями, усечением, конфликтами и без начала соединения', target('cap-3'), { captureIds: ['cap-3'] }, '2026-09-15T16:45:00Z');

  const runs: [string, string, string, string][] = [
    ['run-1', 'ver-1', 'cor-main', '2026-09-14T13:00:00Z'],
    ['run-2', 'ver-1', 'cor-ext', '2026-09-15T10:30:00Z'],
    ['run-3', 'ver-2', 'cor-ext', '2026-09-15T14:00:00Z'],
    ['run-4', 'ver-2', 'cor-def', '2026-09-15T16:50:00Z'],
    ['run-5', 'ver-3', 'cor-ext', '2026-09-16T10:05:00Z'],
  ];
  for (const [id, versionId, corpusId, startedAt] of runs) {
    const stored = createRun(PROJECT_ID, versionId, corpusId, startedAt, id);
    executeRun(stored);
    stored.run.finishedAt = new Date(Date.parse(startedAt) + (stored.run.durationMs ?? 1)).toISOString();
  }

  // Observations are anchored to real field positions of the generated data.
  const fieldRef = (sessionKey: string, ruleId: string, field: string): SourceRef => {
    const sessionId = byKey.get(sessionKey)!;
    const input = sessionInput(sessionId)!;
    const message = parseSession(SPEC_V3, input).find((m) => m.ruleId === ruleId)!;
    const f = message.fields.find((x) => x.name === field)!;
    const offset = message.offset + f.offset;
    return {
      captureId: input.session.captureId,
      captureSha256: input.session.capture.sha256,
      sessionId,
      direction: message.direction,
      offset,
      length: f.length,
      packetNumbers: packetsForRange(sessionId, message.direction, offset, f.length),
    };
  };

  const observations: [string, string, string, string, string, string[], string][] = [
    ['obs-1', 'read-params', 'read_params_resp', 'magic', 'Ответы начинаются с A5, как и запросы', ['h-01'], '2026-09-14T11:18:00Z'],
    ['obs-2', 'set-21', 'set_param_req', 'value', '«Задать 21» → по смещению 6 байт 0x15, следом 0x00', ['h-03', 'h-04'], '2026-09-14T11:58:00Z'],
    ['obs-3', 'set-23', 'set_param_req', 'value', '«Задать 23» → 0x17 0x00', ['h-03', 'h-04'], '2026-09-14T12:02:00Z'],
    ['obs-4', 'read-log', 'read_log_resp', 'length', 'Ответ чтения журнала (167 Б) пришёл в трёх TCP-сегментах; в байтах 3–4 значение 161', ['h-02'], '2026-09-14T13:05:00Z'],
    ['obs-5', 'set-interval', 'set_param_req', 'value', 'Интервал 600 → 58 02: старший байт ненулевой, u8 не подходит', ['h-03', 'h-04'], '2026-09-15T10:35:00Z'],
    ['obs-6', 'set-calibration', 'set_param_req', 'value', 'Калибровка −15 → F1 FF: значение знаковое', ['h-04'], '2026-09-15T10:38:00Z'],
    ['obs-7', 'meas-outdoor', 'get_measurement_resp', 'temperature', '−5.5 °C в клиенте → C9 FF (−55 как i16)', ['h-06'], '2026-09-15T11:10:00Z'],
    ['obs-8', 'meas-2', 'get_measurement_resp', 'uptime', 'Растёт между измерениями: 86412 → 86473', ['h-08'], '2026-09-15T13:28:00Z'],
  ];
  for (const [id, key, rule, field, note, hypothesisIds, createdAt] of observations) {
    const source = fieldRef(key, rule, field);
    db.observations.push({
      id,
      projectId: PROJECT_ID,
      note,
      source,
      snapshotHex: sourceSnapshot(source),
      integrity: 'ok',
      hypothesisIds,
      tags: [],
      createdAt,
    });
  }

  for (const h of HYPOTHESIS_SEEDS) {
    db.hypotheses.push({
      ...h,
      projectId: PROJECT_ID,
      observationIds: db.observations.filter((o) => o.hypothesisIds.includes(h.id)).map((o) => o.id),
      counterexamples:
        h.id === 'h-03'
          ? [
              { source: fieldRef('set-interval', 'set_param_req', 'value'), note: 'Интервал 600: u8 даёт 88', runId: 'run-2' },
              { source: fieldRef('set-calibration', 'set_param_req', 'value'), note: 'Калибровка −15: u8 даёт 241', runId: 'run-2' },
            ]
          : [],
    });
  }

  db.reports.push({
    id: 'rep-1',
    projectId: PROJECT_ID,
    title: 'ThermoLink v2 — промежуточный отчёт',
    createdAt: '2026-09-15T15:00:00Z',
    versionId: 'ver-2',
    version: 2,
    interpretationName: 'ThermoLink',
    runIds: ['run-3'],
    status: 'ready',
    artifacts: [],
    reproduceCommand: '',
  });
}

seed();
