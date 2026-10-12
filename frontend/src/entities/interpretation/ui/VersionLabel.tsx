import { shortHash } from '@/shared/lib';
import { Tag } from '@/shared/ui';

export function VersionLabel({ version, hash, name }: { version: number; hash?: string; name?: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      {name && <span>{name}</span>}
      <Tag>v{version}</Tag>
      {hash && <span className="font-mono text-2xs text-muted">{shortHash(hash, 8)}</span>}
    </span>
  );
}
