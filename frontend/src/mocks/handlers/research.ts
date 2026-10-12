import { http, HttpResponse } from 'msw';
import type { CreateHypothesisInput, Hypothesis, UpdateHypothesisInput } from '@/entities/hypothesis';
import type { CreateObservationInput, Observation } from '@/entities/observation';
import { db, nextId, sourceSnapshot } from '../db';
import { API, apiError, list, notFound, prelude } from './utils';

export const researchHandlers = [
  http.get(`${API}/projects/:projectId/hypotheses`, async ({ params }) => {
    await prelude();
    return list(db.hypotheses.filter((h) => h.projectId === params.projectId).sort((a, b) => a.id.localeCompare(b.id)));
  }),

  http.post(`${API}/projects/:projectId/hypotheses`, async ({ params, request }) => {
    await prelude();
    const input = (await request.json()) as CreateHypothesisInput;
    if (!input.title?.trim()) return apiError(422, 'validation_error', 'Сформулируйте гипотезу');
    const now = new Date().toISOString();
    const h: Hypothesis = {
      id: nextId('h'),
      projectId: String(params.projectId),
      title: input.title.trim(),
      statement: input.statement?.trim() ?? '',
      status: 'proposed',
      observationIds: input.observationIds ?? [],
      counterexamples: [],
      alternatives: input.alternatives ?? [],
      ruleRefs: input.ruleRefs ?? [],
      createdAt: now,
      updatedAt: now,
      history: [{ at: now, status: 'proposed', note: 'Создана' }],
    };
    db.hypotheses.push(h);
    for (const id of h.observationIds) db.observations.find((o) => o.id === id)?.hypothesisIds.push(h.id);
    return HttpResponse.json(h, { status: 201 });
  }),

  http.patch(`${API}/hypotheses/:id`, async ({ params, request }) => {
    await prelude();
    const h = db.hypotheses.find((x) => x.id === params.id);
    if (!h) return notFound('Гипотеза');
    const patch = (await request.json()) as UpdateHypothesisInput;
    const now = new Date().toISOString();
    // As the backend: everything is validated first, a rejected request changes nothing.
    const statusChange = patch.status !== undefined && patch.status !== h.status;
    if (statusChange && !patch.note?.trim()) return apiError(422, 'validation_error', 'Укажите основание смены статуса');
    if (patch.title !== undefined && !patch.title.trim()) return apiError(422, 'validation_error', 'Формулировка не может быть пустой');
    const observationIds = patch.observationIds && [...new Set(patch.observationIds)];
    const missing = observationIds?.find((id) => !db.observations.some((o) => o.id === id));
    if (missing) return apiError(422, 'validation_error', `Наблюдение ${missing} не найдено`);

    if (statusChange && patch.status) {
      h.history.push({ at: now, status: patch.status, note: patch.note!.trim() });
      h.status = patch.status;
    }
    if (patch.title !== undefined) h.title = patch.title.trim();
    if (patch.statement !== undefined) h.statement = patch.statement.trim();
    if (patch.alternatives) h.alternatives = patch.alternatives;
    if (observationIds) {
      // The link is kept on both sides: observations?hypothesisId= reads the observation side.
      for (const o of db.observations) {
        const linked = observationIds.includes(o.id);
        const has = o.hypothesisIds.includes(h.id);
        if (linked && !has) o.hypothesisIds.push(h.id);
        if (!linked && has) o.hypothesisIds = o.hypothesisIds.filter((id) => id !== h.id);
      }
      h.observationIds = observationIds;
    }
    if (patch.counterexamples) h.counterexamples = patch.counterexamples;
    h.updatedAt = now;
    return HttpResponse.json(h);
  }),

  http.get(`${API}/projects/:projectId/observations`, async ({ params, request }) => {
    await prelude();
    const url = new URL(request.url);
    const hypothesisId = url.searchParams.get('hypothesisId');
    const sessionId = url.searchParams.get('sessionId');
    return list(
      db.observations
        .filter((o) => o.projectId === params.projectId)
        .filter((o) => !hypothesisId || o.hypothesisIds.includes(hypothesisId))
        .filter((o) => !sessionId || o.source.sessionId === sessionId)
        .map((o) => ({ ...o, integrity: sourceSnapshot(o.source) === o.snapshotHex ? 'ok' : 'mismatch' }))
        .sort((a, b) => b.createdAt.localeCompare(a.createdAt)),
    );
  }),

  http.post(`${API}/projects/:projectId/observations`, async ({ params, request }) => {
    await prelude();
    const input = (await request.json()) as CreateObservationInput;
    if (!input.note?.trim()) return apiError(422, 'validation_error', 'Опишите наблюдение');
    const session = db.sessions.get(input.source?.sessionId);
    if (!session) return notFound('Сессия');
    if (session.session.capture.sha256 !== input.source.captureSha256) {
      return apiError(409, 'source_mismatch', 'SHA-256 записи не совпадает с источником наблюдения');
    }
    const o: Observation = {
      id: nextId('obs'),
      projectId: String(params.projectId),
      note: input.note.trim(),
      source: input.source,
      snapshotHex: sourceSnapshot(input.source),
      integrity: 'ok',
      hypothesisIds: input.hypothesisIds ?? [],
      tags: input.tags ?? [],
      createdAt: new Date().toISOString(),
    };
    db.observations.push(o);
    for (const id of o.hypothesisIds) db.hypotheses.find((h) => h.id === id)?.observationIds.push(o.id);
    return HttpResponse.json(o, { status: 201 });
  }),

  http.delete(`${API}/observations/:id`, async ({ params }) => {
    await prelude();
    const idx = db.observations.findIndex((o) => o.id === params.id);
    if (idx < 0) return notFound('Наблюдение');
    const [removed] = db.observations.splice(idx, 1);
    for (const h of db.hypotheses) h.observationIds = h.observationIds.filter((id) => id !== removed.id);
    return new HttpResponse(null, { status: 204 });
  }),
];
