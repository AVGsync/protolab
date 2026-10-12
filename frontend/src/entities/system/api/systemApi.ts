import { queryOptions } from '@tanstack/react-query';
import { http } from '@/shared/api';
import type { SystemInfo } from '../model/types';

export const systemApi = {
  info: () => http.get<SystemInfo>('/system/info'),
};

export const systemQueries = {
  info: () => queryOptions({ queryKey: ['system'], queryFn: systemApi.info, refetchInterval: 10_000 }),
};
