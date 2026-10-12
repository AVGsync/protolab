import { http, HttpResponse } from 'msw';
import type { Message } from '@/entities/message';
import type { SessionStatus } from '@/entities/session';
import type { Direction } from '@/shared/api';
import { db, sessionInput, sessionsOf } from '../db';
import { parseSession } from '../engine/interpreter';
import { API, apiError, list, notFound, prelude } from './utils';

const isDirection = (v: unknown): v is Direction => v === 'c2s' || v === 's2c';

export const sessionHandlers = [
  http.get(`${API}/projects/:projectId/sessions`, async ({ params, request }) => {
    await prelude();
    const url = new URL(request.url);
    const captureId = url.searchParams.get('captureId');
    const status = url.searchParams.get('status') as SessionStatus | null;
    const target = url.searchParams.get('target');
    const q = url.searchParams.get('q')?.trim().toLowerCase();

    const items = sessionsOf(String(params.projectId))
      .map((s) => s.session)
      .filter((s) => !captureId || s.captureId === captureId)
      .filter((s) => !status || s.status === status)
      .filter((s) => target === null || String(s.isTarget) === target)
      .filter((s) => {
        if (!q) return true;
        const hay = `${s.id} ${s.client.ip}:${s.client.port} ${s.server.ip}:${s.server.port} ${s.capture.fileName}`.toLowerCase();
        return hay.includes(q);
      })
      .sort((a, b) => a.startedAt.localeCompare(b.startedAt));
    return list(items);
  }),

  http.get(`${API}/sessions/:sessionId`, async ({ params }) => {
    await prelude();
    const s = db.sessions.get(String(params.sessionId));
    return s ? HttpResponse.json(s.session) : notFound('Сессия');
  }),

  http.get(`${API}/sessions/:sessionId/streams/:dir`, async ({ params }) => {
    await prelude();
    const s = db.sessions.get(String(params.sessionId));
    if (!s) return notFound('Сессия');
    if (!isDirection(params.dir)) return apiError(400, 'bad_request', 'Направление: c2s или s2c');
    return HttpResponse.json(s.streams[params.dir].meta);
  }),

  http.get(`${API}/sessions/:sessionId/streams/:dir/bytes`, async ({ params, request }) => {
    await prelude(80);
    const s = db.sessions.get(String(params.sessionId));
    if (!s) return notFound('Сессия');
    if (!isDirection(params.dir)) return apiError(400, 'bad_request', 'Направление: c2s или s2c');
    const url = new URL(request.url);
    const bytes = s.streams[params.dir].bytes;
    const offset = Number(url.searchParams.get('offset') ?? 0);
    const length = Number(url.searchParams.get('length') ?? bytes.length);
    const slice = bytes.slice(offset, offset + length);
    return HttpResponse.arrayBuffer(slice.buffer, {
      headers: { 'Content-Type': 'application/octet-stream', 'X-Stream-Length': String(bytes.length) },
    });
  }),

  http.get(`${API}/sessions/:sessionId/packets`, async ({ params }) => {
    await prelude();
    const s = db.sessions.get(String(params.sessionId));
    return s ? list(s.packets) : notFound('Сессия');
  }),

  http.get(`${API}/sessions/:sessionId/messages`, async ({ params, request }) => {
    await prelude(150);
    const input = sessionInput(String(params.sessionId));
    if (!input) return notFound('Сессия');
    const versionId = new URL(request.url).searchParams.get('versionId');
    const version = db.versions.find((v) => v.id === versionId);
    if (!version) return apiError(400, 'bad_request', 'Укажите versionId интерпретации');
    return list(parseSession(version.spec, input));
  }),

  http.get(`${API}/projects/:projectId/messages`, async ({ params, request }) => {
    await prelude(200);
    const url = new URL(request.url);
    const version = db.versions.find((v) => v.id === url.searchParams.get('versionId'));
    if (!version) return apiError(400, 'bad_request', 'Укажите versionId интерпретации');
    const ruleId = url.searchParams.get('ruleId');
    const direction = url.searchParams.get('direction');
    const sessionIds = url.searchParams.get('sessionIds')?.split(',').filter(Boolean);

    const sessions = sessionsOf(String(params.projectId))
      .filter((s) => s.session.isTarget)
      .filter((s) => !sessionIds?.length || sessionIds.includes(s.session.id))
      .sort((a, b) => a.session.startedAt.localeCompare(b.session.startedAt));

    const items: Message[] = [];
    for (const s of sessions) {
      const input = sessionInput(s.session.id)!;
      for (const m of parseSession(version.spec, input)) {
        if (ruleId && m.ruleId !== ruleId) continue;
        if (direction && m.direction !== direction) continue;
        items.push(m);
      }
    }
    return list(items.slice(0, 2000));
  }),
];
