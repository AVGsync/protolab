import { Outlet } from 'react-router';
import { AppHeader } from '@/widgets/app-header';
import { Toaster } from '@/shared/ui';

export function RootLayout() {
  return (
    <div className="flex h-full flex-col">
      <AppHeader />
      <main className="min-h-0 flex-1 overflow-auto">
        <Outlet />
      </main>
      <Toaster />
    </div>
  );
}
