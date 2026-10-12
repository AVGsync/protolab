import { http, HttpResponse } from 'msw';
import type { CreateReportInput, Report } from '@/entities/report';
import type { SystemInfo } from '@/entities/system';
import { db, nextId } from '../db';
import { ENGINE_VERSION } from '../engine/interpreter';
import { cancelJob, getJob, listJobs, startJob } from '../jobs';
import { renderArtifact, reportDto } from '../report';
import { API, apiError, list, notFound, prelude } from './utils';

const startedAt = Date.now();

export const systemHandlers = [
  http.get(`${API}/jobs`, async ({ request }) => {
    await prelude(60);
    const limit = Number(new URL(request.url).searchParams.get('limit') ?? 20);
    return list(listJobs(limit));
  }),

  http.get(`${API}/jobs/:jobId`, async ({ params }) => {
    await prelude(60);
    const job = getJob(String(params.jobId));
    return job ? HttpResponse.json(job) : notFound('Задача');
  }),

  http.post(`${API}/jobs/:jobId/cancel`, async ({ params }) => {
    await prelude(60);
    const job = cancelJob(String(params.jobId));
    if (!job) return notFound('Задача');
    if (job.status !== 'cancelled') return apiError(409, 'not_cancellable', 'Задача уже завершена');
    return HttpResponse.json(job);
  }),

  http.get(`${API}/projects/:projectId/reports`, async ({ params }) => {
    await prelude();
    return list(
      db.reports
        .filter((r) => r.projectId === params.projectId)
        .map(reportDto)
        .sort((a, b) => b.createdAt.localeCompare(a.createdAt)),
    );
  }),

  http.post(`${API}/projects/:projectId/reports`, async ({ params, request }) => {
    await prelude();
    const projectId = String(params.projectId);
    const input = (await request.json()) as CreateReportInput;
    const version = db.versions.find((v) => v.id === input.versionId);
    if (!version) return notFound('Версия интерпретации');
    const interp = db.interpretations.find((i) => i.id === version.interpretationId)!;
    const report: Report = {
      id: nextId('rep'),
      projectId,
      title: input.title?.trim() || `${interp.name} v${version.version}`,
      createdAt: new Date().toISOString(),
      versionId: version.id,
      version: version.version,
      interpretationName: interp.name,
      runIds: input.runIds ?? [],
      status: 'pending',
      artifacts: [],
      reproduceCommand: '',
    };
    db.reports.push(report);
    const job = startJob({
      projectId,
      kind: 'report',
      title: `Отчёт «${report.title}»`,
      durationMs: 2000,
      stages: ['Сбор оснований', 'Рендер отчёта', 'Экспорт интерпретации и примеров'],
      onDone: () => {
        report.status = 'ready';
        return { type: 'report', id: report.id };
      },
      onCancel: () => {
        report.status = 'failed';
      },
    });
    return HttpResponse.json({ report: reportDto(report), jobId: job.id }, { status: 202 });
  }),

  http.get(`${API}/reports/:reportId/files/:fileName`, async ({ params }) => {
    await prelude();
    const report = db.reports.find((r) => r.id === params.reportId);
    if (!report) return notFound('Отчёт');
    const fileName = String(params.fileName);
    const content = renderArtifact(report, fileName);
    if (content === undefined) return notFound('Файл');
    return new HttpResponse(content, {
      headers: {
        'Content-Type': fileName.endsWith('.md') ? 'text/markdown; charset=utf-8' : 'application/json',
        'Content-Disposition': `attachment; filename="${fileName}"`,
      },
    });
  }),

  http.get(`${API}/reports/:reportId/bundle`, async ({ params }) => {
    await prelude(200);
    const report = db.reports.find((r) => r.id === params.reportId);
    if (!report) return notFound('Отчёт');
    if (report.status !== 'ready') return apiError(409, 'not_ready', 'Отчёт ещё не сформирован');
    // The real backend sends a zip of the artifacts plus the captures of the examples.
    const files = Object.fromEntries(reportDto(report).artifacts.map((a) => [a.fileName, renderArtifact(report, a.fileName)]));
    const bundle = { note: 'Mock-комплект (JSON). Реальный backend отдаёт .zip с файлами отчёта и исходными записями примеров.', files };
    return new HttpResponse(JSON.stringify(bundle, null, 2), {
      headers: { 'Content-Type': 'application/json', 'Content-Disposition': `attachment; filename="${report.id}.bundle.json"` },
    });
  }),

  http.get(`${API}/system/info`, async () => {
    await prelude(60);
    const used = [...db.sessions.values()].reduce((n, s) => n + s.streams.c2s.bytes.length + s.streams.s2c.bytes.length, 0);
    const info: SystemInfo = {
      version: '0.1.0-mock',
      commit: 'mock',
      buildDate: '2026-10-12',
      engineVersion: ENGINE_VERSION,
      // The defaults of the Go backend (internal/app.DefaultLimits).
      limits: {
        maxCaptureBytes: 512 * 1024 * 1024,
        maxPackets: 2_000_000,
        maxSessions: 20_000,
        maxMessages: 200_000,
        maxMessageBytes: 65_600,
        maxStreamBytes: 64 * 1024 * 1024,
        maxTotalStreamBytes: 1024 * 1024 * 1024,
      },
      runtime: {
        os: 'linux',
        arch: 'amd64',
        cpus: 4,
        goVersion: 'go1.25 (mock)',
        memAllocBytes: 38_000_000 + Math.round(Math.random() * 4_000_000),
        memSysBytes: 96_000_000,
        uptimeSec: Math.round((Date.now() - startedAt) / 1000),
      },
      storage: { dataDir: './data', usedBytes: db.captures.reduce((n, c) => n + c.sizeBytes, 0) + used },
    };
    return HttpResponse.json(info);
  }),
];
