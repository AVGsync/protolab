import { keepPreviousData, queryOptions } from '@tanstack/react-query';
import { http, type ListResponse } from '@/shared/api';
import type { SessionFilters, TcpSession } from '../model/types';

export const sessionApi = {
  list: (projectId: string, filters: SessionFilters = {}) =>
    http.get<ListResponse<TcpSession>>(`/projects/${projectId}/sessions`, { ...filters }),
  get: (sessionId: string) => http.get<TcpSession>(`/sessions/${sessionId}`),
};

export const sessionQueries = {
  list: (projectId: string, filters: SessionFilters = {}) =>
    queryOptions({
      queryKey: ['project', projectId, 'sessions', filters],
      queryFn: () => sessionApi.list(projectId, filters),
      placeholderData: keepPreviousData,
    }),
  detail: (sessionId: string) =>
    queryOptions({ queryKey: ['session', sessionId, 'detail'], queryFn: () => sessionApi.get(sessionId) }),
};
