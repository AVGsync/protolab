import { createBrowserRouter, Navigate } from 'react-router';
import { Loader } from '@/shared/ui';
import { SectionLayout } from '../layouts/SectionLayout';
import { dataTabs, specTabs } from '../layouts/section-tabs';
import { RootLayout } from '../layouts/RootLayout';
import { RouteError } from '../layouts/RouteError';

/**
 * Pages are code-split: each route loads its slice on demand.
 * A project has six numbered sections (see PROJECT_SECTIONS); sections with sub-tabs share a layout.
 */
export const router = createBrowserRouter([
  {
    path: '/',
    element: <RootLayout />,
    errorElement: <RouteError />,
    hydrateFallbackElement: <Loader />,
    children: [
      { index: true, lazy: () => import('@/pages/projects').then((m) => ({ Component: m.ProjectsPage })) },
      { path: 'system', lazy: () => import('@/pages/system').then((m) => ({ Component: m.SystemPage })) },
      {
        path: 'projects/:projectId',
        errorElement: <RouteError />,
        children: [
          { index: true, element: <Navigate to="data" replace /> },
          {
            path: 'data',
            element: <SectionLayout tabs={dataTabs} />,
            children: [
              { index: true, lazy: () => import('@/pages/captures').then((m) => ({ Component: m.CapturesPage })) },
              { path: 'actions', lazy: () => import('@/pages/actions').then((m) => ({ Component: m.ActionsPage })) },
            ],
          },
          { path: 'sessions', lazy: () => import('@/pages/sessions').then((m) => ({ Component: m.SessionsPage })) },
          {
            path: 'sessions/:sessionId',
            lazy: () => import('@/pages/session-workspace').then((m) => ({ Component: m.SessionWorkspacePage })),
          },
          { path: 'compare', lazy: () => import('@/pages/compare').then((m) => ({ Component: m.ComparePage })) },
          {
            path: 'spec',
            element: <SectionLayout tabs={specTabs} />,
            children: [
              {
                index: true,
                lazy: () => import('@/pages/interpretation-editor').then((m) => ({ Component: m.InterpretationEditorPage })),
              },
              { path: 'hypotheses', lazy: () => import('@/pages/hypotheses').then((m) => ({ Component: m.HypothesesPage })) },
              { path: 'observations', lazy: () => import('@/pages/observations').then((m) => ({ Component: m.ObservationsPage })) },
            ],
          },
          { path: 'verification', lazy: () => import('@/pages/verification').then((m) => ({ Component: m.VerificationPage })) },
          { path: 'verification/:runId', lazy: () => import('@/pages/run-details').then((m) => ({ Component: m.RunDetailsPage })) },
          { path: 'report', lazy: () => import('@/pages/reports').then((m) => ({ Component: m.ReportsPage })) },
          { path: '*', lazy: () => import('@/pages/not-found').then((m) => ({ Component: m.NotFoundPage })) },
        ],
      },
      { path: '*', lazy: () => import('@/pages/not-found').then((m) => ({ Component: m.NotFoundPage })) },
    ],
  },
]);
