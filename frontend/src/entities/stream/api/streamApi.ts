import { queryOptions } from '@tanstack/react-query';
import { http, type Direction, type ListResponse } from '@/shared/api';
import type { DirectedStream, Packet } from '../model/types';

export const streamApi = {
  get: (sessionId: string, direction: Direction) =>
    http.get<DirectedStream>(`/sessions/${sessionId}/streams/${direction}`),
  /**
   * application/octet-stream. Gap bytes are zero-filled in transport only —
   * always consult DirectedStream.gaps before interpreting them.
   */
  bytes: (sessionId: string, direction: Direction, range?: { offset: number; length: number }) =>
    http.bytes(`/sessions/${sessionId}/streams/${direction}/bytes`, range),
  packets: (sessionId: string) => http.get<ListResponse<Packet>>(`/sessions/${sessionId}/packets`),
};

export const streamQueries = {
  meta: (sessionId: string, direction: Direction) =>
    queryOptions({
      queryKey: ['session', sessionId, 'stream', direction],
      queryFn: () => streamApi.get(sessionId, direction),
    }),
  bytes: (sessionId: string, direction: Direction) =>
    queryOptions({
      queryKey: ['session', sessionId, 'bytes', direction],
      queryFn: () => streamApi.bytes(sessionId, direction),
      staleTime: Infinity,
    }),
  packets: (sessionId: string) =>
    queryOptions({ queryKey: ['session', sessionId, 'packets'], queryFn: () => streamApi.packets(sessionId) }),
};
