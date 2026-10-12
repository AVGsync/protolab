import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import {
  HYPOTHESIS_STATUS,
  HYPOTHESIS_STATUSES,
  hypothesisApi,
  type Hypothesis,
  type HypothesisStatus,
} from '@/entities/hypothesis';
import { projectKeys } from '@/entities/project';
import { Button, Field, Input, Select, toast } from '@/shared/ui';

/** Status changes always carry a reason and are kept in the hypothesis history. */
export function HypothesisStatusForm({ hypothesis }: { hypothesis: Hypothesis }) {
  const [status, setStatus] = useState<HypothesisStatus>(hypothesis.status);
  const [note, setNote] = useState('');
  const qc = useQueryClient();

  const mutation = useMutation({
    mutationFn: () => hypothesisApi.update(hypothesis.id, { status, note }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: projectKeys.root(hypothesis.projectId) });
      toast.success('Статус гипотезы обновлён');
      setNote('');
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  const changed = status !== hypothesis.status;

  return (
    <form onSubmit={submit} className="grid grid-cols-[160px_1fr_auto] items-end gap-2">
      <Field label="Статус">
        <Select value={status} onChange={(e) => setStatus(e.target.value as HypothesisStatus)}>
          {HYPOTHESIS_STATUSES.map((s) => (
            <option key={s} value={s}>
              {HYPOTHESIS_STATUS[s].label}
            </option>
          ))}
        </Select>
      </Field>
      <Field label="Основание">
        <Input
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder={changed ? 'Какие данные и проверки привели к изменению' : 'Выберите новый статус'}
          disabled={!changed}
        />
      </Field>
      <Button type="submit" variant="primary" loading={mutation.isPending} disabled={!changed || !note.trim()}>
        Применить
      </Button>
    </form>
  );
}
