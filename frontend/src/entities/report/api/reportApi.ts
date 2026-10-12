import { queryOptions } from '@tanstack/react-query';
import { http, type ListResponse } from '@/shared/api';
import type { CreateReportInput, Report } from '../model/types';

export const reportApi = {
  list: (projectId: string) => http.get<ListResponse<Report>>(`/projects/${projectId}/reports`),
  create: (projectId: string, input: CreateReportInput) =>
    http.post<{ report: Report; jobId: string }>(`/projects/${projectId}/reports`, input),
};

export const reportQueries = {
  list: (projectId: string) =>
    queryOptions({ queryKey: ['project', projectId, 'reports'], queryFn: () => reportApi.list(projectId) }),
};
