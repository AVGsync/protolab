import { http, HttpResponse } from 'msw';
import type { Capture } from '@/entities/capture';
import { db, nextId } from '../db';
import { generateCapture } from '../fixtures/generator';
import { sha256Hex } from '../fixtures/hash';
import { UPLOAD_TEMPLATE } from '../fixtures/scenarios';
import { startJob } from '../jobs';
import { linkPendingActions } from './actions';
import { API, apiError, list, notFound, prelude } from './utils';

/** libpcap (both byte orders, µs/ns) and pcapng section header magics. */
function detectFormat(head: Uint8Array): 'pcap' | 'pcapng' | null {
  const hex = [...head.subarray(0, 4)].map((b) => b.toString(16).padStart(2, '0')).join('');
  if (['d4c3b2a1', 'a1b2c3d4', '4d3cb2a1', 'a1b23c4d'].includes(hex)) return 'pcap';
  if (hex === '0a0d0d0a') return 'pcapng';
  return null;
}

export const captureHandlers = [
  http.get(`${API}/projects/:projectId/captures`, async ({ params }) => {
    await prelude();
    return list(db.captures.filter((c) => c.projectId === params.projectId));
  }),

  http.get(`${API}/captures/:captureId`, async ({ params }) => {
    await prelude();
    const c = db.captures.find((x) => x.id === params.captureId);
    return c ? HttpResponse.json(c) : notFound('Запись');
  }),

  http.post(`${API}/projects/:projectId/captures`, async ({ params, request }) => {
    await prelude(250);
    const project = db.projects.find((p) => p.id === params.projectId);
    if (!project) return notFound('Проект');
    const file = (await request.formData()).get('file');
    if (!(file instanceof File)) return apiError(400, 'bad_request', 'Файл не передан');

    const buf = new Uint8Array(await file.arrayBuffer());
    const format = detectFormat(buf);
    if (!format) {
      return apiError(422, 'unsupported_format', 'Файл не является PCAP/PCAPNG (неизвестная сигнатура)', {
        fileName: file.name,
      });
    }
    const sha256 = await sha256Hex(buf);
    const duplicate = db.captures.find((c) => c.projectId === project.id && c.sha256 === sha256);
    if (duplicate) {
      return apiError(409, 'duplicate_capture', `Эта запись уже импортирована как ${duplicate.fileName}`, { captureId: duplicate.id });
    }

    const id = nextId('cap');
    const capture: Capture = {
      id,
      projectId: project.id,
      fileName: file.name,
      format,
      sizeBytes: file.size,
      sha256,
      integrity: 'ok',
      linkType: 'EN10MB',
      snapLen: 262144,
      interfaces: 1,
      packetCount: 0,
      sessionCount: 0,
      targetSessionCount: 0,
      firstPacketAt: null,
      lastPacketAt: null,
      importedAt: new Date().toISOString(),
      status: 'indexing',
      diagnostics: [],
    };
    db.captures.push(capture);

    const job = startJob({
      projectId: project.id,
      kind: 'import_capture',
      title: `Импорт ${file.name}`,
      durationMs: 3500,
      stages: ['Чтение пакетов', 'Сборка TCP-сессий', 'Восстановление потоков', 'Индексация'],
      onDone: () => {
        // Mock mode does not parse the file: sessions are generated from a template.
        const generated = generateCapture(
          {
            id,
            projectId: project.id,
            fileName: file.name,
            format,
            startIso: new Date(Date.now() - 600_000).toISOString().replace(/\.\d+Z$/, 'Z'),
            importedAt: capture.importedAt,
            snapLen: 262144,
            targetPorts: project.settings.targetPorts,
            sessions: UPLOAD_TEMPLATE.map((s) => ({ ...s, actionId: undefined })),
          },
          (_key, i) => `${id}-s${String(i + 1).padStart(2, '0')}`,
        );
        Object.assign(capture, {
          ...generated.capture,
          fileName: file.name,
          sha256,
          sizeBytes: file.size,
          importedAt: capture.importedAt,
          diagnostics: [
            { level: 'info', code: 'mock_template', message: 'Mock-режим: содержимое файла не разбирается, сессии сгенерированы по шаблону' },
            ...generated.capture.diagnostics,
          ],
        });
        for (const s of generated.sessions) {
          s.session.capture = { fileName: file.name, sha256 };
          db.sessions.set(s.session.id, s);
        }
        linkPendingActions(project.id);
        return { type: 'capture', id };
      },
      onCancel: () => {
        capture.status = 'error';
        capture.diagnostics.push({ level: 'error', code: 'cancelled', message: 'Импорт отменён пользователем' });
      },
    });
    capture.jobId = job.id;

    return HttpResponse.json({ capture, jobId: job.id }, { status: 202 });
  }),
];
