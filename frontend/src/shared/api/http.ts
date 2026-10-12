import { env } from '../config/env';

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: unknown;

  constructor(status: number, code: string, message: string, details?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

type QueryValue = string | number | boolean | null | undefined | readonly (string | number)[];
export type Query = Record<string, QueryValue>;

export function apiUrl(path: string, query?: Query): string {
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value === undefined || value === null || value === '') continue;
    if (Array.isArray(value)) {
      if (value.length > 0) qs.set(key, value.join(','));
    } else {
      qs.set(key, String(value));
    }
  }
  const s = qs.toString();
  return `${env.apiBaseUrl}${path}${s ? `?${s}` : ''}`;
}

interface RequestOptions {
  query?: Query;
  body?: unknown;
  signal?: AbortSignal;
}

async function send(method: string, path: string, opts: RequestOptions): Promise<Response> {
  const isForm = opts.body instanceof FormData;
  const hasBody = opts.body !== undefined;

  let res: Response;
  try {
    res = await fetch(apiUrl(path, opts.query), {
      method,
      signal: opts.signal,
      headers: hasBody && !isForm ? { 'Content-Type': 'application/json' } : undefined,
      body: !hasBody ? undefined : isForm ? (opts.body as FormData) : JSON.stringify(opts.body),
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err;
    throw new ApiError(0, 'network_error', 'Сервер недоступен. Проверьте, что backend запущен.');
  }

  if (!res.ok) throw await toApiError(res);
  return res;
}

/** Error envelope: {"error": {"code": "...", "message": "...", "details": ...}} */
async function toApiError(res: Response): Promise<ApiError> {
  try {
    const payload = (await res.json()) as { error?: { code?: string; message?: string; details?: unknown } };
    if (payload.error) {
      return new ApiError(
        res.status,
        payload.error.code ?? 'unknown',
        payload.error.message ?? res.statusText,
        payload.error.details,
      );
    }
  } catch {
    // non-JSON body — fall through
  }
  return new ApiError(res.status, 'http_error', `HTTP ${res.status}: ${res.statusText}`);
}

async function json<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  const res = await send(method, path, opts);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const http = {
  get: <T>(path: string, query?: Query, signal?: AbortSignal) => json<T>('GET', path, { query, signal }),
  post: <T>(path: string, body?: unknown) => json<T>('POST', path, { body }),
  put: <T>(path: string, body?: unknown) => json<T>('PUT', path, { body }),
  patch: <T>(path: string, body?: unknown) => json<T>('PATCH', path, { body }),
  delete: <T = void>(path: string) => json<T>('DELETE', path),
  /** Raw binary payload (application/octet-stream). */
  bytes: async (path: string, query?: Query, signal?: AbortSignal): Promise<Uint8Array> => {
    const res = await send('GET', path, { query, signal });
    return new Uint8Array(await res.arrayBuffer());
  },
};

export function formData(fields: Record<string, string | Blob | undefined>): FormData {
  const fd = new FormData();
  for (const [key, value] of Object.entries(fields)) {
    if (value !== undefined) fd.append(key, value);
  }
  return fd;
}
