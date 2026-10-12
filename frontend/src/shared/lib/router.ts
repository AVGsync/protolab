import { useParams } from 'react-router';

/** Route params that must exist for the page to render (guaranteed by the route config). */
export function useRequiredParams<K extends string>(...keys: K[]): Record<K, string> {
  const params = useParams();
  const out = {} as Record<K, string>;
  for (const key of keys) {
    const value = params[key];
    if (!value) throw new Error(`Route parameter "${key}" is missing`);
    out[key] = value;
  }
  return out;
}
