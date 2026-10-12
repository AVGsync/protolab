import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { hypothesisApi, type Hypothesis } from '@/entities/hypothesis';
import { observationQueries } from '@/entities/observation';
import { projectKeys } from '@/entities/project';
import { Button, Select, toast } from '@/shared/ui';

/** Links an existing observation to the hypothesis as a basis (the observation keeps its source bytes). */
export function LinkObservationForm({ hypothesis: h }: { hypothesis: Hypothesis }) {
  const [observationId, setObservationId] = useState('');
  const qc = useQueryClient();
  const observations = useQuery(observationQueries.list(h.projectId));
  const candidates = (observations.data?.items ?? []).filter((o) => !h.observationIds.includes(o.id));

  const mutation = useMutation({
    mutationFn: () => hypothesisApi.update(h.id, { observationIds: [...h.observationIds, observationId] }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: projectKeys.root(h.projectId) });
      toast.success('Наблюдение связано с гипотезой');
      setObservationId('');
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  if (observations.isSuccess && candidates.length === 0) return null;
  return (
    <form onSubmit={submit} className="mt-2 flex items-center gap-2">
      <Select value={observationId} onChange={(e) => setObservationId(e.target.value)} className="flex-1" aria-label="Наблюдение">
        <option value="">Связать наблюдение…</option>
        {candidates.map((o) => (
          <option key={o.id} value={o.id}>
            {o.note} ({o.source.sessionId}/{o.source.direction})
          </option>
        ))}
      </Select>
      <Button type="submit" size="sm" loading={mutation.isPending} disabled={!observationId}>
        Связать
      </Button>
    </form>
  );
}

/** Removes the link between an observation and the hypothesis; the observation itself stays. */
export function UnlinkObservationButton({ hypothesis: h, observationId }: { hypothesis: Hypothesis; observationId: string }) {
  const qc = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => hypothesisApi.update(h.id, { observationIds: h.observationIds.filter((id) => id !== observationId) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: projectKeys.root(h.projectId) }),
    onError: (err) => toast.fromError(err),
  });
  return (
    <Button size="xs" variant="ghost" loading={mutation.isPending} onClick={() => mutation.mutate()} title="Убрать из оснований гипотезы">
      отвязать
    </Button>
  );
}
