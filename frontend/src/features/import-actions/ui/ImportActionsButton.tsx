import { useMutation, useQueryClient } from '@tanstack/react-query';
import { actionApi } from '@/entities/action';
import { projectKeys } from '@/entities/project';
import { FileButton, toast } from '@/shared/ui';

/** CSV columns: timestamp,kind,label,params,result. JSON: array of actions. */
export function ImportActionsButton({ projectId }: { projectId: string }) {
  const qc = useQueryClient();
  const mutation = useMutation({
    mutationFn: (file: File) => actionApi.import(projectId, file),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      toast.success(`Импортировано действий: ${res.imported}, связано с сессиями: ${res.linked}`);
      if (res.warnings.length) toast.info(res.warnings.slice(0, 3).join('\n'));
    },
    onError: (err) => toast.fromError(err),
  });

  return (
    <FileButton accept=".csv,.json" loading={mutation.isPending} onFile={(f) => mutation.mutate(f)}>
      Импорт журнала
    </FileButton>
  );
}
