import type { CorpusFilter } from './types';

/** Human-readable corpus conditions, e.g. for verification summaries. */
export function describeCorpusFilter(filter: CorpusFilter): string[] {
  const parts: string[] = [];
  if (filter.captureIds?.length) parts.push(`записи: ${filter.captureIds.join(', ')}`);
  if (filter.onlyTarget) parts.push('только целевой порт');
  if (filter.onlyComplete) parts.push('только полные сессии');
  if (filter.actionKinds?.length) parts.push(`действия: ${filter.actionKinds.join(', ')}`);
  return parts.length ? parts : ['без фильтра — сессии выбраны вручную'];
}
