import { http, HttpResponse } from 'msw';
import type { CreateCorpusInput } from '@/entities/corpus';
import type { FindingKind, StartRunInput } from '@/entities/verification';
import { corpusHash, createRun, db, executeRun, nextId, runDto } from '../db';
import { startJob } from '../jobs';
import { API, apiError, list, notFound, prelude } from './utils';

export const verificationHandlers = [
  http.get(`${API}/projects/:projectId/corpora`, async ({ params }) => {
    await prelude();
    return list(db.corpora.filter((c) => c.projectId === params.projectId));
  }),

  http.post(`${API}/projects/:projectId/corpora`, async ({ params, request }) => {
    await prelude();
    const input = (await request.json()) as CreateCorpusInput;
    if (!input.name?.trim()) return apiError(422, 'validation_error', 'Укажите название корпуса');
    if (!input.sessionIds?.length) return apiError(422, 'validation_error', 'Корпус должен содержать хотя бы одну сессию');
    const missing = input.sessionIds.filter((id) => !db.sessions.has(id));
    if (missing.length) return apiError(422, 'validation_error', `Неизвестные сессии: ${missing.join(', ')}`);
    const corpus = {
      id: nextId('cor'),
      projectId: String(params.projectId),
      name: input.name.trim(),
      description: input.description ?? '',
      sessionIds: input.sessionIds,
      filter: input.filter ?? {},
      hash: corpusHash(input.sessionIds),
      createdAt: new Date().toISOString(),
    };
    db.corpora.push(corpus);
    return HttpResponse.json(corpus, { status: 201 });
  }),

  http.get(`${API}/projects/:projectId/runs`, async ({ params }) => {
    await prelude();
    return list(
      db.runs
        .filter((r) => r.run.projectId === params.projectId)
        .map(runDto)
        .sort((a, b) => b.startedAt.localeCompare(a.startedAt)),
    );
  }),

  http.post(`${API}/projects/:projectId/runs`, async ({ params, request }) => {
    await prelude();
    const projectId = String(params.projectId);
    const input = (await request.json()) as StartRunInput;
    const version = db.versions.find((v) => v.id === input.versionId);
    const corpus = db.corpora.find((c) => c.id === input.corpusId && c.projectId === projectId);
    if (!version) return notFound('Версия интерпретации');
    if (!corpus) return notFound('Корпус');

    const stored = createRun(projectId, version.id, corpus.id, new Date().toISOString());
    stored.run.status = 'running';
    const job = startJob({
      projectId,
      kind: 'verification',
      title: `Проверка v${version.version} на «${corpus.name}»`,
      durationMs: 1500 + corpus.sessionIds.length * 150,
      stages: ['Загрузка корпуса', 'Кадрирование', 'Применение правил', 'Сводка'],
      onDone: () => {
        executeRun(stored);
        stored.run.finishedAt = new Date().toISOString();
        return { type: 'run', id: stored.run.id };
      },
      onCancel: () => {
        stored.run.status = 'cancelled';
        stored.run.finishedAt = new Date().toISOString();
      },
    });
    stored.run.jobId = job.id;
    return HttpResponse.json({ run: runDto(stored), jobId: job.id }, { status: 202 });
  }),

  http.get(`${API}/runs/:runId`, async ({ params }) => {
    await prelude();
    const r = db.runs.find((x) => x.run.id === params.runId);
    return r ? HttpResponse.json(runDto(r)) : notFound('Проверка');
  }),

  http.get(`${API}/runs/:runId/findings`, async ({ params, request }) => {
    await prelude();
    const r = db.runs.find((x) => x.run.id === params.runId);
    if (!r) return notFound('Проверка');
    const kind = new URL(request.url).searchParams.get('kind') as FindingKind | null;
    return list(r.findings.filter((f) => !kind || f.kind === kind));
  }),
];
