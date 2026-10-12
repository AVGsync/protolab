import { queryOptions } from '@tanstack/react-query';
import { http, type ListResponse } from '@/shared/api';
import { isJobActive, type Job } from '../model/types';

export const jobApi = {
  list: () => http.get<ListResponse<Job>>('/jobs', { limit: 20 }),
  get: (jobId: string) => http.get<Job>(`/jobs/${jobId}`),
  cancel: (jobId: string) => http.post<Job>(`/jobs/${jobId}/cancel`),
};

export const jobKeys = { all: ['jobs'] as const };

export const jobQueries = {
  /** Polls fast while anything is running, slowly otherwise. */
  list: () =>
    queryOptions({
      queryKey: jobKeys.all,
      queryFn: jobApi.list,
      refetchInterval: (query) => (query.state.data?.items.some(isJobActive) ? 700 : 5000),
    }),
};
