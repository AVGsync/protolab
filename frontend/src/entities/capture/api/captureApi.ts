import { queryOptions } from '@tanstack/react-query';
import { formData, http, type ListResponse } from '@/shared/api';
import type { Capture, UploadCaptureResult } from '../model/types';

export const captureApi = {
  list: (projectId: string) => http.get<ListResponse<Capture>>(`/projects/${projectId}/captures`),
  get: (captureId: string) => http.get<Capture>(`/captures/${captureId}`),
  /** multipart/form-data, field "file". Indexing runs as a job. */
  upload: (projectId: string, file: File) =>
    http.post<UploadCaptureResult>(`/projects/${projectId}/captures`, formData({ file })),
};

export const captureQueries = {
  list: (projectId: string) =>
    queryOptions({
      queryKey: ['project', projectId, 'captures'],
      queryFn: () => captureApi.list(projectId),
    }),
};
