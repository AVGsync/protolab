import { queryOptions } from '@tanstack/react-query';
import type { Message } from '@/entities/message';
import { apiUrl, http, type ListResponse } from '@/shared/api';
import type {
  CreateInterpretationInput,
  Interpretation,
  InterpretationSpec,
  InterpretationVersion,
  ValidationResult,
} from '../model/types';

export const interpretationApi = {
  list: (projectId: string) => http.get<ListResponse<Interpretation>>(`/projects/${projectId}/interpretations`),
  get: (interpretationId: string) => http.get<Interpretation>(`/interpretations/${interpretationId}`),
  create: (projectId: string, input: CreateInterpretationInput) =>
    http.post<Interpretation>(`/projects/${projectId}/interpretations`, input),
  listVersions: (interpretationId: string) =>
    http.get<ListResponse<InterpretationVersion>>(`/interpretations/${interpretationId}/versions`),
  getVersion: (versionId: string) => http.get<InterpretationVersion>(`/interpretation-versions/${versionId}`),
  createVersion: (interpretationId: string, input: { spec: InterpretationSpec; comment: string }) =>
    http.post<InterpretationVersion>(`/interpretations/${interpretationId}/versions`, input),
  validate: (spec: unknown) => http.post<ValidationResult>('/interpretations/validate', { spec }),
  /** Apply an unsaved spec to a session without creating a version. */
  preview: (spec: InterpretationSpec, sessionId: string) =>
    http.post<ListResponse<Message>>('/interpretations/preview', { spec, sessionId }),
  /** Machine-readable spec download. */
  exportUrl: (versionId: string) => apiUrl(`/interpretation-versions/${versionId}/export`),
};

export const interpretationQueries = {
  list: (projectId: string) =>
    queryOptions({
      queryKey: ['project', projectId, 'interpretations'],
      queryFn: () => interpretationApi.list(projectId),
    }),
  detail: (interpretationId: string) =>
    queryOptions({
      queryKey: ['interpretation', interpretationId, 'detail'],
      queryFn: () => interpretationApi.get(interpretationId),
    }),
  versions: (interpretationId: string) =>
    queryOptions({
      queryKey: ['interpretation', interpretationId, 'versions'],
      queryFn: () => interpretationApi.listVersions(interpretationId),
    }),
  version: (versionId: string) =>
    queryOptions({
      queryKey: ['interpretation-version', versionId],
      queryFn: () => interpretationApi.getVersion(versionId),
      staleTime: Infinity,
    }),
};
