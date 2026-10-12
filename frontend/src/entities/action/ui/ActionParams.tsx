export function ActionParams({ params }: { params: Record<string, string | number> | undefined }) {
  const entries = Object.entries(params ?? {});
  if (entries.length === 0) return <span className="text-subtle">—</span>;
  return (
    <span className="inline-flex flex-wrap gap-x-2.5 gap-y-0.5 font-mono text-xs">
      {entries.map(([k, v]) => (
        <span key={k}>
          <span className="text-muted">{k}=</span>
          {v}
        </span>
      ))}
    </span>
  );
}
