import { Outlet } from 'react-router';
import { AppHeader } from '@/widgets/app-header';
import { SoftKeys } from '@/widgets/soft-keys';
import { Toaster } from '@/shared/ui';

export function RootLayout() {
  return (
    <div className="bg-grain flex h-full flex-col">
      <AppHeader />
      <main className="min-h-0 flex-1 overflow-auto">
        <Outlet />
      </main>
      <SoftKeys />
      <Toaster />
    </div>
  );
}
