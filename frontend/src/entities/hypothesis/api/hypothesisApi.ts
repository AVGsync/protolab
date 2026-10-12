import { queryOptions } from '@tanstack/react-query';
import { http, type ListResponse } from '@/shared/api';
import type { CreateHypothesisInput, Hypothesis, UpdateHypothesisInput } from '../model/types';

export const hypothesisApi = {
  list: (projectId: string) => http.get<ListResponse<Hypothesis>>(`/projects/${projectId}/hypotheses`),
  create: (projectId: string, input: CreateHypothesisInput) =>
    http.post<Hypothesis>(`/projects/${projectId}/hypotheses`, input),
  update: (hypothesisId: string, input: UpdateHypothesisInput) =>
    http.patch<Hypothesis>(`/hypotheses/${hypothesisId}`, input),
};

export const hypothesisQueries = {
  list: (projectId: string) =>
    queryOptions({ queryKey: ['project', projectId, 'hypotheses'], queryFn: () => hypothesisApi.list(projectId) }),
};
