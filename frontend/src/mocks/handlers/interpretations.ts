import { http, HttpResponse } from 'msw';
import type { CreateInterpretationInput, Interpretation, InterpretationSpec, InterpretationVersion } from '@/entities/interpretation';
import { db, nextId, sessionInput, specHash } from '../db';
import { parseSession, validateSpec } from '../engine/interpreter';
import { API, apiError, list, notFound, prelude } from './utils';

/**
 * As the backend: a downloaded spec file carries "$meta" (dropped on load), and omitted lists
 * become empty ones, so the stored spec and its hash have one canonical form.
 */
function normalizeSpec(input: unknown): InterpretationSpec {
  if (typeof input !== 'object' || input === null || Array.isArray(input)) return input as InterpretationSpec;
  const { $meta: _meta, ...spec } = input as InterpretationSpec & { $meta?: unknown };
  return {
    ...spec,
    messages: (spec.messages ?? []).map((m) => ({ ...m, match: m.match ?? [], fields: m.fields ?? [] })),
    ...(spec.assertions && { assertions: spec.assertions.map((a) => ({ ...a, checks: a.checks ?? [] })) }),
  };
}

function invalid(spec: unknown) {
  const result = validateSpec(spec);
  return result.valid ? null : apiError(422, 'invalid_spec', 'Интерпретация содержит ошибки', result.issues);
}

export const interpretationHandlers = [
  http.get(`${API}/projects/:projectId/interpretations`, async ({ params }) => {
    await prelude();
    return list(db.interpretations.filter((i) => i.projectId === params.projectId));
  }),

  http.post(`${API}/projects/:projectId/interpretations`, async ({ params, request }) => {
    await prelude();
    const body = (await request.json()) as CreateInterpretationInput;
    const input = { ...body, spec: normalizeSpec(body.spec) };
    if (!input.name?.trim()) return apiError(422, 'validation_error', 'Укажите название');
    const error = invalid(input.spec);
    if (error) return error;
    const now = new Date().toISOString();
    const interp: Interpretation = {
      id: nextId('int'),
      projectId: String(params.projectId),
      name: input.name.trim(),
      description: input.description ?? '',
      latestVersion: 1,
      latestVersionId: '',
      createdAt: now,
      updatedAt: now,
    };
    const version: InterpretationVersion = {
      id: nextId('ver'),
      interpretationId: interp.id,
      version: 1,
      hash: specHash(input.spec),
      comment: 'Первая версия',
      createdAt: now,
      spec: input.spec,
    };
    interp.latestVersionId = version.id;
    db.interpretations.push(interp);
    db.versions.push(version);
    return HttpResponse.json(interp, { status: 201 });
  }),

  http.post(`${API}/interpretations/validate`, async ({ request }) => {
    await prelude(60);
    const { spec } = (await request.json()) as { spec: unknown };
    return HttpResponse.json(validateSpec(normalizeSpec(spec)));
  }),

  http.post(`${API}/interpretations/preview`, async ({ request }) => {
    await prelude(150);
    const body = (await request.json()) as { spec: unknown; sessionId: string };
    const spec = normalizeSpec(body.spec);
    const sessionId = body.sessionId;
    const error = invalid(spec);
    if (error) return error;
    const input = sessionInput(sessionId);
    if (!input) return notFound('Сессия');
    return list(parseSession(spec, input));
  }),

  http.get(`${API}/interpretations/:id`, async ({ params }) => {
    await prelude();
    const i = db.interpretations.find((x) => x.id === params.id);
    return i ? HttpResponse.json(i) : notFound('Интерпретация');
  }),

  http.get(`${API}/interpretations/:id/versions`, async ({ params }) => {
    await prelude();
    return list(db.versions.filter((v) => v.interpretationId === params.id).sort((a, b) => b.version - a.version));
  }),

  http.post(`${API}/interpretations/:id/versions`, async ({ params, request }) => {
    await prelude();
    const interp = db.interpretations.find((x) => x.id === params.id);
    if (!interp) return notFound('Интерпретация');
    const body = (await request.json()) as { spec: unknown; comment: string };
    const spec = normalizeSpec(body.spec);
    const comment = body.comment;
    const error = invalid(spec);
    if (error) return error;
    const hash = specHash(spec);
    const latest = db.versions.find((v) => v.id === interp.latestVersionId);
    if (latest?.hash === hash) return apiError(409, 'no_changes', 'Спецификация не отличается от последней версии');
    const version: InterpretationVersion = {
      id: nextId('ver'),
      interpretationId: interp.id,
      version: interp.latestVersion + 1,
      hash,
      comment: comment?.trim() || 'Без комментария',
      createdAt: new Date().toISOString(),
      parentVersionId: interp.latestVersionId,
      spec,
    };
    db.versions.push(version);
    interp.latestVersion = version.version;
    interp.latestVersionId = version.id;
    interp.updatedAt = version.createdAt;
    return HttpResponse.json(version, { status: 201 });
  }),

  http.get(`${API}/interpretation-versions/:versionId`, async ({ params }) => {
    await prelude();
    const v = db.versions.find((x) => x.id === params.versionId);
    return v ? HttpResponse.json(v) : notFound('Версия');
  }),

  http.get(`${API}/interpretation-versions/:versionId/export`, async ({ params }) => {
    await prelude();
    const v = db.versions.find((x) => x.id === params.versionId);
    if (!v) return notFound('Версия');
    const interp = db.interpretations.find((i) => i.id === v.interpretationId)!;
    const body = JSON.stringify({ ...v.spec, $meta: { name: interp.name, version: v.version, hash: v.hash } }, null, 2);
    return new HttpResponse(body, {
      headers: {
        'Content-Type': 'application/json',
        'Content-Disposition': `attachment; filename="${interp.name.toLowerCase()}.v${v.version}.json"`,
      },
    });
  }),
];
