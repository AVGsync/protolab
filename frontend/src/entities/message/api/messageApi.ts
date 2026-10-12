import { keepPreviousData, queryOptions } from '@tanstack/react-query';
import { http, type ListResponse } from '@/shared/api';
import type { Message, ProjectMessagesQuery } from '../model/types';

export const messageApi = {
  /** Messages of a session framed and parsed by an interpretation version. */
  listBySession: (sessionId: string, versionId: string) =>
    http.get<ListResponse<Message>>(`/sessions/${sessionId}/messages`, { versionId }),
  /** Messages across sessions — used to compare examples. */
  listByProject: (projectId: string, query: ProjectMessagesQuery) =>
    http.get<ListResponse<Message>>(`/projects/${projectId}/messages`, { ...query }),
};

export const messageQueries = {
  bySession: (sessionId: string, versionId: string) =>
    queryOptions({
      queryKey: ['session', sessionId, 'messages', versionId],
      queryFn: () => messageApi.listBySession(sessionId, versionId),
    }),
  byProject: (projectId: string, query: ProjectMessagesQuery) =>
    queryOptions({
      queryKey: ['project', projectId, 'messages', query],
      queryFn: () => messageApi.listByProject(projectId, query),
      placeholderData: keepPreviousData,
    }),
};
