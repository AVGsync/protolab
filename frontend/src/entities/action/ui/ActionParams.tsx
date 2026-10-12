export function ActionParams({ params }: { params: Record<string, string | number> | undefined }) {
  const entries = Object.entries(params ?? {});
  if (entries.length === 0) return <span className="text-subtle">—</span>;
  return (
    <span className="inline-flex flex-wrap gap-1">
      {entries.map(([k, v]) => (
        <span key={k} className="rounded border border-line bg-surface-2 px-1.5 font-mono text-[11px]">
          <span className="text-muted">{k}=</span>
          <span className="text-glow">{v}</span>
        </span>
      ))}
    </span>
  );
}
