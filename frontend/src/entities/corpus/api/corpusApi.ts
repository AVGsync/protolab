import { queryOptions } from '@tanstack/react-query';
import { http, type ListResponse } from '@/shared/api';
import type { Corpus, CreateCorpusInput } from '../model/types';

export const corpusApi = {
  list: (projectId: string) => http.get<ListResponse<Corpus>>(`/projects/${projectId}/corpora`),
  create: (projectId: string, input: CreateCorpusInput) => http.post<Corpus>(`/projects/${projectId}/corpora`, input),
};

export const corpusQueries = {
  list: (projectId: string) =>
    queryOptions({ queryKey: ['project', projectId, 'corpora'], queryFn: () => corpusApi.list(projectId) }),
};
