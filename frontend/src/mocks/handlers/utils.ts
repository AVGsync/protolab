import { delay, HttpResponse } from 'msw';
import { tickJobs } from '../jobs';

export const API = '/api/v1';

export function apiError(status: number, code: string, message: string, details?: unknown) {
  return HttpResponse.json({ error: { code, message, details } }, { status });
}

export const notFound = (what: string) => apiError(404, 'not_found', `${what} не найден(а)`);

export function list<T>(items: T[]) {
  return HttpResponse.json({ items, total: items.length });
}

/** Common prelude: simulated latency + advancing background jobs. */
export async function prelude(ms = 120): Promise<void> {
  await delay(ms);
  tickJobs();
}
