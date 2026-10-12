import { http, HttpResponse } from 'msw';
import type { CreateProjectInput, Project, ProjectSettings } from '@/entities/project';
import { db, nextId, projectWithStats, sessionsOf } from '../db';
import { startJob } from '../jobs';
import { API, apiError, list, notFound, prelude } from './utils';

export const projectHandlers = [
  http.get(`${API}/projects`, async () => {
    await prelude();
    return list(db.projects.map(projectWithStats));
  }),

  http.post(`${API}/projects`, async ({ request }) => {
    await prelude();
    const input = (await request.json()) as CreateProjectInput;
    if (!input.name?.trim()) return apiError(422, 'validation_error', 'Укажите название проекта');
    const now = new Date().toISOString();
    const project: Project = {
      id: nextId('prj'),
      name: input.name.trim(),
      description: input.description?.trim() ?? '',
      createdAt: now,
      updatedAt: now,
      settings: { overlapPolicy: 'first', checksumPolicy: 'warn', targetPorts: input.targetPorts ?? [] },
      stats: { captures: 0, sessions: 0, actions: 0, observations: 0, hypotheses: 0, interpretations: 0, runs: 0 },
    };
    db.projects.unshift(project);
    return HttpResponse.json(projectWithStats(project), { status: 201 });
  }),

  http.post(`${API}/projects/import`, async ({ request }) => {
    await prelude(300);
    const file = (await request.formData()).get('file');
    if (!(file instanceof File)) return apiError(400, 'bad_request', 'Файл проекта не передан');
    const now = new Date().toISOString();
    const project: Project = {
      id: nextId('prj'),
      name: file.name.replace(/\.(zip|json)$/i, '').replace(/\.protolab$/i, ''),
      description: `Импортирован из ${file.name}`,
      createdAt: now,
      updatedAt: now,
      settings: { overlapPolicy: 'first', checksumPolicy: 'warn', targetPorts: [] },
      stats: { captures: 0, sessions: 0, actions: 0, observations: 0, hypotheses: 0, interpretations: 0, runs: 0 },
    };
    db.projects.unshift(project);
    startJob({
      projectId: project.id,
      kind: 'import_project',
      title: `Импорт проекта ${file.name}`,
      durationMs: 2500,
      stages: ['Распаковка архива', 'Проверка SHA-256 исходных записей', 'Восстановление индексов'],
      onDone: () => ({ type: 'project', id: project.id }),
    });
    return HttpResponse.json(projectWithStats(project), { status: 201 });
  }),

  http.get(`${API}/projects/:projectId`, async ({ params }) => {
    await prelude();
    const p = db.projects.find((x) => x.id === params.projectId);
    return p ? HttpResponse.json(projectWithStats(p)) : notFound('Проект');
  }),

  http.patch(`${API}/projects/:projectId`, async ({ params, request }) => {
    await prelude();
    const p = db.projects.find((x) => x.id === params.projectId);
    if (!p) return notFound('Проект');
    const body = (await request.json()) as { settings?: Partial<ProjectSettings>; name?: string; description?: string };
    if (body.settings) p.settings = { ...p.settings, ...body.settings };
    if (body.name) p.name = body.name;
    if (body.description !== undefined) p.description = body.description;
    p.updatedAt = new Date().toISOString();
    for (const s of sessionsOf(p.id)) s.session.isTarget = p.settings.targetPorts.includes(s.session.server.port);
    return HttpResponse.json(projectWithStats(p));
  }),

  http.get(`${API}/projects/:projectId/export`, async ({ params }) => {
    await prelude(200);
    const p = db.projects.find((x) => x.id === params.projectId);
    if (!p) return notFound('Проект');
    const bundle = {
      format: 'protolab-project/v1',
      note: 'Mock-экспорт (JSON). Реальный backend отдаёт .zip с исходными файлами записей.',
      project: projectWithStats(p),
      captures: db.captures.filter((c) => c.projectId === p.id).map((c) => ({ ...c, path: `captures/${c.sha256}.${c.format}` })),
      actions: db.actions.filter((a) => a.projectId === p.id),
      interpretations: db.interpretations.filter((i) => i.projectId === p.id),
      versions: db.versions.filter((v) => db.interpretations.some((i) => i.id === v.interpretationId && i.projectId === p.id)),
      hypotheses: db.hypotheses.filter((h) => h.projectId === p.id),
      observations: db.observations.filter((o) => o.projectId === p.id),
      corpora: db.corpora.filter((c) => c.projectId === p.id),
      runs: db.runs.filter((r) => r.run.projectId === p.id).map((r) => r.run),
    };
    return new HttpResponse(JSON.stringify(bundle, null, 2), {
      headers: {
        'Content-Type': 'application/json',
        'Content-Disposition': `attachment; filename="${p.id}.protolab.json"`,
      },
    });
  }),
];
