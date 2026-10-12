import type { Direction } from '../api/types';
import { serializeRange, type ByteRange } from '../lib/ranges';

export interface SessionRouteParams {
  dir?: Direction;
  /** Interpretation version used for framing. */
  v?: string;
  /** Selected byte range in the directed stream. */
  sel?: ByteRange;
}

function withQuery(path: string, query: Record<string, string | undefined>): string {
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== '') qs.set(key, value);
  }
  const s = qs.toString();
  return s ? `${path}?${s}` : path;
}

const project = (projectId: string) => `/projects/${projectId}`;

/**
 * Single source of truth for application URLs.
 * A project has six sections: data, sessions, compare, spec, verification, report.
 */
export const routes = {
  home: () => '/',
  system: () => '/system',
  /** The project root redirects to the data section. */
  project,
  /** 1. Данные: captures (index) and the action log. */
  data: (projectId: string) => `${project(projectId)}/data`,
  actions: (projectId: string) => `${project(projectId)}/data/actions`,
  /** 2. Сессии: the list and the workspace of one session. */
  sessions: (projectId: string, query: { captureId?: string } = {}) =>
    withQuery(`${project(projectId)}/sessions`, query),
  session: (projectId: string, sessionId: string, params: SessionRouteParams = {}) =>
    withQuery(`${project(projectId)}/sessions/${sessionId}`, {
      dir: params.dir,
      v: params.v,
      sel: params.sel ? serializeRange(params.sel) : undefined,
    }),
  /** 3. Сравнение. */
  compare: (projectId: string) => `${project(projectId)}/compare`,
  /** 4. Спецификация: the editor (index) with hypotheses and observations next to it. */
  spec: (projectId: string, target: { interpretationId?: string; versionId?: string } = {}) =>
    withQuery(`${project(projectId)}/spec`, { i: target.interpretationId, v: target.versionId }),
  hypotheses: (projectId: string, hypothesisId?: string) =>
    withQuery(`${project(projectId)}/spec/hypotheses`, { h: hypothesisId }),
  observations: (projectId: string) => `${project(projectId)}/spec/observations`,
  /** 5. Проверка: corpora with runs, and the details of a run. */
  verification: (projectId: string) => `${project(projectId)}/verification`,
  run: (projectId: string, runId: string) => `${project(projectId)}/verification/${runId}`,
  /** 6. Отчёт: reports, export and project settings. */
  report: (projectId: string) => `${project(projectId)}/report`,
} as const;
