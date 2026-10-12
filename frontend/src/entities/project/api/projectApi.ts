import { queryOptions } from '@tanstack/react-query';
import { apiUrl, formData, http, type ListResponse } from '@/shared/api';
import type { CreateProjectInput, Project, ProjectSettings } from '../model/types';

export const projectApi = {
  list: () => http.get<ListResponse<Project>>('/projects'),
  get: (projectId: string) => http.get<Project>(`/projects/${projectId}`),
  create: (input: CreateProjectInput) => http.post<Project>('/projects', input),
  updateSettings: (projectId: string, settings: Partial<ProjectSettings>) =>
    http.patch<Project>(`/projects/${projectId}`, { settings }),
  import: (file: File) => http.post<Project>('/projects/import', formData({ file })),
  /** Portable project bundle (.zip): captures, interpretations, observations, runs. */
  exportUrl: (projectId: string) => apiUrl(`/projects/${projectId}/export`),
};

export const projectKeys = {
  list: () => ['projects'] as const,
  /** Root key of everything that belongs to a project — invalidate it after jobs finish. */
  root: (projectId: string) => ['project', projectId] as const,
};

export const projectQueries = {
  list: () => queryOptions({ queryKey: projectKeys.list(), queryFn: projectApi.list }),
  detail: (projectId: string) =>
    queryOptions({ queryKey: [...projectKeys.root(projectId), 'detail'], queryFn: () => projectApi.get(projectId) }),
};
