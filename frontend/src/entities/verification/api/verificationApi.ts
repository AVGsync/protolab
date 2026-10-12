import { queryOptions } from '@tanstack/react-query';
import { http, type ListResponse } from '@/shared/api';
import type { Finding, FindingKind, StartRunInput, StartRunResult, VerificationRun } from '../model/types';

export const verificationApi = {
  listRuns: (projectId: string) => http.get<ListResponse<VerificationRun>>(`/projects/${projectId}/runs`),
  getRun: (runId: string) => http.get<VerificationRun>(`/runs/${runId}`),
  listFindings: (runId: string, kind?: FindingKind) =>
    http.get<ListResponse<Finding>>(`/runs/${runId}/findings`, { kind }),
  start: (projectId: string, input: StartRunInput) => http.post<StartRunResult>(`/projects/${projectId}/runs`, input),
};

export const verificationQueries = {
  runs: (projectId: string) =>
    queryOptions({ queryKey: ['project', projectId, 'runs'], queryFn: () => verificationApi.listRuns(projectId) }),
  run: (runId: string) => queryOptions({ queryKey: ['run', runId, 'detail'], queryFn: () => verificationApi.getRun(runId) }),
  findings: (runId: string, kind?: FindingKind) =>
    queryOptions({
      queryKey: ['run', runId, 'findings', kind ?? 'all'],
      queryFn: () => verificationApi.listFindings(runId, kind),
    }),
};
