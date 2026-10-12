import { shortHash } from '@/shared/lib';

export function VersionLabel({ version, hash, name }: { version: number; hash?: string; name?: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      {name && <span>{name}</span>}
      <span className="rounded bg-surface-3 px-1.5 font-mono text-[11px] text-fg">v{version}</span>
      {hash && <span className="font-mono text-[11px] text-subtle">{shortHash(hash, 8)}</span>}
    </span>
  );
}
