import { isRouteErrorResponse, Link, useRouteError } from 'react-router';
import { routes } from '@/shared/config';
import { buttonClass, ErrorState } from '@/shared/ui';

export function RouteError() {
  const error = useRouteError();
  const normalized = isRouteErrorResponse(error) ? new Error(`${error.status} ${error.statusText}`) : error;
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4">
      <ErrorState error={normalized} />
      <Link to={routes.home()} className={buttonClass('secondary')}>
        К проектам
      </Link>
    </div>
  );
}
