import { queryOptions } from '@tanstack/react-query';
import { http, type ListResponse } from '@/shared/api';
import type { CreateObservationInput, Observation, ObservationFilters } from '../model/types';

export const observationApi = {
  list: (projectId: string, filters: ObservationFilters = {}) =>
    http.get<ListResponse<Observation>>(`/projects/${projectId}/observations`, { ...filters }),
  create: (projectId: string, input: CreateObservationInput) =>
    http.post<Observation>(`/projects/${projectId}/observations`, input),
  remove: (observationId: string) => http.delete(`/observations/${observationId}`),
};

export const observationQueries = {
  list: (projectId: string, filters: ObservationFilters = {}) =>
    queryOptions({
      queryKey: ['project', projectId, 'observations', filters],
      queryFn: () => observationApi.list(projectId, filters),
    }),
};
