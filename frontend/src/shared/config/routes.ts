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

/** Single source of truth for application URLs. */
export const routes = {
  home: () => '/',
  system: () => '/system',
  project,
  captures: (projectId: string) => `${project(projectId)}/captures`,
  sessions: (projectId: string, query: { captureId?: string } = {}) =>
    withQuery(`${project(projectId)}/sessions`, query),
  session: (projectId: string, sessionId: string, params: SessionRouteParams = {}) =>
    withQuery(`${project(projectId)}/sessions/${sessionId}`, {
      dir: params.dir,
      v: params.v,
      sel: params.sel ? serializeRange(params.sel) : undefined,
    }),
  compare: (projectId: string) => `${project(projectId)}/compare`,
  actions: (projectId: string) => `${project(projectId)}/actions`,
  interpretations: (projectId: string) => `${project(projectId)}/interpretations`,
  interpretation: (projectId: string, interpretationId: string, versionId?: string) =>
    withQuery(`${project(projectId)}/interpretations/${interpretationId}`, { v: versionId }),
  hypotheses: (projectId: string, hypothesisId?: string) =>
    withQuery(`${project(projectId)}/hypotheses`, { h: hypothesisId }),
  observations: (projectId: string) => `${project(projectId)}/observations`,
  verification: (projectId: string) => `${project(projectId)}/verification`,
  run: (projectId: string, runId: string) => `${project(projectId)}/verification/${runId}`,
  reports: (projectId: string) => `${project(projectId)}/reports`,
} as const;
