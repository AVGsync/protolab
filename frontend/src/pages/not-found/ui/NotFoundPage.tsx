import { Link } from 'react-router';
import { routes } from '@/shared/config';
import { buttonClass, EmptyState } from '@/shared/ui';

export function NotFoundPage() {
  return (
    <EmptyState
      className="py-24"
      title="Страница не найдена"
      description="Возможно, ссылка устарела или объект удалён."
      action={
        <Link to={routes.home()} className={buttonClass('secondary')}>
          К проектам
        </Link>
      }
    />
  );
}
