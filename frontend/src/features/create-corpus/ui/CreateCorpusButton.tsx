import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState, type FormEvent } from 'react';
import { ACTION_KIND_LABEL, actionQueries } from '@/entities/action';
import { captureQueries } from '@/entities/capture';
import { corpusApi, type CorpusFilter } from '@/entities/corpus';
import { projectKeys } from '@/entities/project';
import { EndpointPair, SessionStatusMark, sessionQueries } from '@/entities/session';
import { formatTimestamp } from '@/shared/lib';
import { Button, Checkbox, Field, Input, Modal, Table, TBody, Td, Th, THead, Tr, toast } from '@/shared/ui';

/** A corpus is an explicit, hashed list of sessions plus the conditions it was built with. */
export function CreateCorpusButton({ projectId }: { projectId: string }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [captureIds, setCaptureIds] = useState<string[]>([]);
  const [onlyTarget, setOnlyTarget] = useState(true);
  const [onlyComplete, setOnlyComplete] = useState(false);
  const [excluded, setExcluded] = useState<string[]>([]);
  const qc = useQueryClient();

  const sessions = useQuery({ ...sessionQueries.list(projectId), enabled: open });
  const captures = useQuery({ ...captureQueries.list(projectId), enabled: open });
  const actions = useQuery({ ...actionQueries.list(projectId), enabled: open });

  const actionLabel = useMemo(() => {
    const map = new Map<string, string>();
    for (const a of actions.data?.items ?? []) for (const l of a.links) map.set(l.sessionId, `${ACTION_KIND_LABEL[a.kind]}: ${a.label}`);
    return map;
  }, [actions.data]);

  const candidates = useMemo(
    () =>
      (sessions.data?.items ?? [])
        .filter((s) => captureIds.length === 0 || captureIds.includes(s.captureId))
        .filter((s) => !onlyTarget || s.isTarget)
        .filter((s) => !onlyComplete || s.status === 'complete'),
    [sessions.data, captureIds, onlyTarget, onlyComplete],
  );
  const selected = candidates.filter((s) => !excluded.includes(s.id));

  const mutation = useMutation({
    mutationFn: () => {
      const filter: CorpusFilter = {
        captureIds: captureIds.length ? captureIds : undefined,
        onlyTarget: onlyTarget || undefined,
        onlyComplete: onlyComplete || undefined,
      };
      return corpusApi.create(projectId, { name, description, sessionIds: selected.map((s) => s.id), filter });
    },
    onSuccess: (corpus) => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      toast.success(`Корпус «${corpus.name}»: ${corpus.sessionIds.length} сессий`);
      setOpen(false);
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  const toggleCapture = (id: string) =>
    setCaptureIds((ids) => (ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id]));

  return (
    <>
      <Button onClick={() => setOpen(true)}>
        Новый корпус
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        size="xl"
        title="Новый корпус"
        footer={
          <>
            <span className="mr-auto self-center text-xs text-muted">Выбрано сессий: {selected.length}</span>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="create-corpus" loading={mutation.isPending} disabled={!name.trim() || selected.length === 0}>
              Создать корпус
            </Button>
          </>
        }
      >
        <form id="create-corpus" onSubmit={submit} className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-4">
            <Field label="Название">
              <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Все изменения настроек" />
            </Field>
            <Field label="Описание">
              <Input value={description} onChange={(e) => setDescription(e.target.value)} />
            </Field>
          </div>
          <div className="flex flex-wrap gap-x-5 gap-y-2 border border-line bg-bg px-3 py-2.5">
            <span className="text-xs text-muted">Записи:</span>
            {captures.data?.items.map((c) => (
              <Checkbox key={c.id} label={c.fileName} checked={captureIds.includes(c.id)} onChange={() => toggleCapture(c.id)} />
            ))}
            <span className="mx-1 w-px self-stretch bg-line" />
            <Checkbox label="только целевой порт" checked={onlyTarget} onChange={(e) => setOnlyTarget(e.target.checked)} />
            <Checkbox label="только полные сессии" checked={onlyComplete} onChange={(e) => setOnlyComplete(e.target.checked)} />
          </div>
          <div className="max-h-80 overflow-y-auto border border-line">
            <Table>
              <THead>
                <tr>
                  <Th className="w-8" />
                  <Th>Сессия</Th>
                  <Th>Соединение</Th>
                  <Th>Начало</Th>
                  <Th>Действие</Th>
                  <Th>Статус</Th>
                </tr>
              </THead>
              <TBody>
                {candidates.map((s) => (
                  <Tr key={s.id}>
                    <Td>
                      <input
                        type="checkbox"
                        className="accent-[var(--color-accent)]"
                        checked={!excluded.includes(s.id)}
                        onChange={() => setExcluded((ids) => (ids.includes(s.id) ? ids.filter((x) => x !== s.id) : [...ids, s.id]))}
                      />
                    </Td>
                    <Td className="font-mono text-xs">{s.id}</Td>
                    <Td>
                      <EndpointPair client={s.client} server={s.server} />
                    </Td>
                    <Td className="font-mono text-xs text-muted">{formatTimestamp(s.startedAt, 3)}</Td>
                    <Td className="text-muted">{actionLabel.get(s.id) ?? '—'}</Td>
                    <Td>
                      <SessionStatusMark status={s.status} />
                    </Td>
                  </Tr>
                ))}
              </TBody>
            </Table>
          </div>
        </form>
      </Modal>
    </>
  );
}
