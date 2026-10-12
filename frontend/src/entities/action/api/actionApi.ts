import { queryOptions } from '@tanstack/react-query';
import { formData, http, type ListResponse } from '@/shared/api';
import type { ActionEntry, CreateActionInput, ImportActionsResult } from '../model/types';

export const actionApi = {
  list: (projectId: string) => http.get<ListResponse<ActionEntry>>(`/projects/${projectId}/actions`),
  create: (projectId: string, input: CreateActionInput) =>
    http.post<ActionEntry>(`/projects/${projectId}/actions`, input),
  /** CSV or JSON action log; sessions are linked by timestamp. */
  import: (projectId: string, file: File) =>
    http.post<ImportActionsResult>(`/projects/${projectId}/actions/import`, formData({ file })),
};

export const actionQueries = {
  list: (projectId: string) =>
    queryOptions({ queryKey: ['project', projectId, 'actions'], queryFn: () => actionApi.list(projectId) }),
};
