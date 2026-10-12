import { useMutation, useQueryClient } from '@tanstack/react-query';
import { X } from 'lucide-react';
import { jobApi, jobKeys, type Job } from '@/entities/job';
import { Button, toast } from '@/shared/ui';

export function CancelJobButton({ job }: { job: Job }) {
  const qc = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => jobApi.cancel(job.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: jobKeys.all });
      toast.info(`«${job.title}» отменена. Уже готовые данные остаются доступны.`);
    },
    onError: (err) => toast.fromError(err),
  });
  if (!job.cancellable) return null;
  return (
    <Button size="xs" variant="ghost" icon={<X size={12} />} loading={mutation.isPending} onClick={() => mutation.mutate()}>
      Отменить
    </Button>
  );
}
