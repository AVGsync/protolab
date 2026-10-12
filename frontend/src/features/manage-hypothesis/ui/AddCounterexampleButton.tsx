import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { HYPOTHESIS_STATUS, hypothesisApi, hypothesisQueries } from '@/entities/hypothesis';
import { projectKeys } from '@/entities/project';
import type { SourceRef } from '@/shared/api';
import { formatRange } from '@/shared/lib';
import { Button, Field, Input, Modal, Mono, Note, Select, toast } from '@/shared/ui';

export interface AddCounterexampleButtonProps {
  projectId: string;
  /** The concrete bytes that contradict the hypothesis (a finding example). */
  source: SourceRef;
  /** The verification run that found them. */
  runId?: string;
  defaultNote?: string;
  /** Hypothesis behind the rule or field of the finding, if the spec names one. */
  defaultHypothesisId?: string;
}

/** Records concrete source bytes as a counterexample of a hypothesis, with the run that found them. */
export function AddCounterexampleButton({ projectId, source, runId, defaultNote = '', defaultHypothesisId }: AddCounterexampleButtonProps) {
  const [open, setOpen] = useState(false);
  const [hypothesisId, setHypothesisId] = useState('');
  const [note, setNote] = useState(defaultNote);
  const qc = useQueryClient();
  const hypotheses = useQuery({ ...hypothesisQueries.list(projectId), enabled: open });
  const items = hypotheses.data?.items ?? [];
  const effective = hypothesisId || (items.some((h) => h.id === defaultHypothesisId) ? defaultHypothesisId! : '');
  const target = items.find((h) => h.id === effective);
  const duplicate = target?.counterexamples.some(
    (c) => c.source.sessionId === source.sessionId && c.source.direction === source.direction && c.source.offset === source.offset,
  );

  const mutation = useMutation({
    mutationFn: () => hypothesisApi.update(target!.id, { counterexamples: [...target!.counterexamples, { source, note: note.trim(), runId }] }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      toast.success(`Контрпример добавлен к гипотезе ${target!.id}`);
      setOpen(false);
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <>
      <Button size="xs" variant="ghost" onClick={() => setOpen(true)} title="Отметить эти байты как контрпример гипотезы">
        контрпример
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Контрпример к гипотезе"
        description={`${source.sessionId}/${source.direction} ${formatRange(source)}`}
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="add-counterexample" loading={mutation.isPending} disabled={!target || !note.trim() || duplicate}>
              Добавить
            </Button>
          </>
        }
      >
        <form id="add-counterexample" onSubmit={submit} className="flex flex-col gap-4">
          <Field label="Гипотеза">
            <Select value={effective} onChange={(e) => setHypothesisId(e.target.value)}>
              <option value="">Выберите гипотезу…</option>
              {items.map((h) => (
                <option key={h.id} value={h.id}>
                  {h.title} ({HYPOTHESIS_STATUS[h.status].label})
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Почему это контрпример">
            <Input value={note} onChange={(e) => setNote(e.target.value)} />
          </Field>
          {runId && (
            <p className="text-xs text-muted">
              Найден проверкой <Mono>{runId}</Mono>; ссылка на байты сохраняется вместе с SHA-256 записи.
            </p>
          )}
          {duplicate && <Note tone="info">Эти байты уже записаны контрпримером этой гипотезы.</Note>}
        </form>
      </Modal>
    </>
  );
}
