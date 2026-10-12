import { http, HttpResponse } from 'msw';
import type { ActionEntry, ActionKind, CreateActionInput } from '@/entities/action';
import { db, nextId, sessionsOf } from '../db';
import { API, apiError, list, prelude } from './utils';

const KINDS: ActionKind[] = ['read_params', 'set_param', 'get_measurement', 'read_log', 'other'];
const LINK_WINDOW_MS = 5000;

/** Links an action to target sessions that start within a few seconds after it. */
export function autoLink(projectId: string, timestamp: string): ActionEntry['links'] {
  const t = Date.parse(timestamp);
  return sessionsOf(projectId)
    .filter((s) => s.session.isTarget)
    .map((s) => ({ s, delta: Date.parse(s.session.startedAt) - t }))
    .filter(({ delta }) => delta >= -500 && delta <= LINK_WINDOW_MS)
    .sort((a, b) => a.delta - b.delta)
    .slice(0, 1)
    .map(({ s, delta }) => ({ sessionId: s.session.id, method: 'auto_time' as const, deltaMs: delta }));
}

function parseParams(raw: string): Record<string, string | number> {
  const out: Record<string, string | number> = {};
  if (!raw.trim()) return out;
  if (raw.trim().startsWith('{')) return JSON.parse(raw) as Record<string, string | number>;
  for (const pair of raw.split(';')) {
    const [k, v] = pair.split('=').map((x) => x.trim());
    if (!k) continue;
    out[k] = v !== '' && Number.isFinite(Number(v)) ? Number(v) : v;
  }
  return out;
}

/** CSV: timestamp,kind,label,params,result  (params: "k=v;k=v" or JSON). */
function parseCsv(text: string): CreateActionInput[] {
  const lines = text.split(/\r?\n/).filter((l) => l.trim());
  const header = lines[0]?.toLowerCase().startsWith('timestamp') ? 1 : 0;
  return lines.slice(header).map((line, i) => {
    const cells = line.match(/("([^"]|"")*"|[^,]*)(,|$)/g)?.map((c) => c.replace(/,$/, '').replace(/^"|"$/g, '').replace(/""/g, '"')) ?? [];
    const [timestamp, kind, label, params = '', result = ''] = cells;
    if (!timestamp || Number.isNaN(Date.parse(timestamp))) throw new Error(`Строка ${i + 1 + header}: некорректное время "${timestamp}"`);
    return {
      timestamp: new Date(timestamp).toISOString(),
      kind: (KINDS.includes(kind as ActionKind) ? kind : 'other') as ActionKind,
      label: label || kind,
      params: parseParams(params),
      result: result || undefined,
    };
  });
}

/** As the backend: actions without a session (log imported before the capture) are linked when a capture is indexed. */
export function linkPendingActions(projectId: string): void {
  for (const action of db.actions) {
    if (action.projectId !== projectId || action.links.length > 0) continue;
    action.links = autoLink(projectId, action.timestamp);
    for (const l of action.links) db.sessions.get(l.sessionId)?.session.actionIds.push(action.id);
  }
}

export const actionHandlers = [
  http.get(`${API}/projects/:projectId/actions`, async ({ params }) => {
    await prelude();
    return list(db.actions.filter((a) => a.projectId === params.projectId).sort((a, b) => a.timestamp.localeCompare(b.timestamp)));
  }),

  http.post(`${API}/projects/:projectId/actions`, async ({ params, request }) => {
    await prelude();
    const projectId = String(params.projectId);
    const input = (await request.json()) as CreateActionInput;
    if (!input.label?.trim() || Number.isNaN(Date.parse(input.timestamp))) {
      return apiError(422, 'validation_error', 'Укажите время и описание действия');
    }
    const action: ActionEntry = {
      id: nextId('act'),
      projectId,
      timestamp: input.timestamp,
      kind: input.kind,
      label: input.label.trim(),
      params: input.params ?? {},
      observed: input.observed,
      result: input.result,
      source: 'manual',
      links: input.sessionIds?.length
        ? input.sessionIds.map((sessionId) => ({ sessionId, method: 'manual' as const }))
        : autoLink(projectId, input.timestamp),
    };
    db.actions.push(action);
    for (const l of action.links) db.sessions.get(l.sessionId)?.session.actionIds.push(action.id);
    return HttpResponse.json(action, { status: 201 });
  }),

  http.post(`${API}/projects/:projectId/actions/import`, async ({ params, request }) => {
    await prelude(300);
    const projectId = String(params.projectId);
    const file = (await request.formData()).get('file');
    if (!(file instanceof File)) return apiError(400, 'bad_request', 'Файл не передан');
    const text = await file.text();
    let inputs: CreateActionInput[];
    try {
      inputs = file.name.endsWith('.json') ? (JSON.parse(text) as CreateActionInput[]) : parseCsv(text);
    } catch (err) {
      return apiError(422, 'invalid_action_log', err instanceof Error ? err.message : 'Не удалось разобрать журнал');
    }
    const warnings: string[] = [];
    let linked = 0;
    for (const input of inputs) {
      const links = autoLink(projectId, input.timestamp);
      if (links.length) linked++;
      else warnings.push(`«${input.label}» (${input.timestamp}): сессия в окне ${LINK_WINDOW_MS / 1000} с не найдена`);
      const action: ActionEntry = { ...input, id: nextId('act'), projectId, source: 'import', links };
      db.actions.push(action);
      for (const l of links) db.sessions.get(l.sessionId)?.session.actionIds.push(action.id);
    }
    return HttpResponse.json({ imported: inputs.length, linked, warnings });
  }),
];
