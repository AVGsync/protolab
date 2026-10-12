import js from '@eslint/js';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import globals from 'globals';
import tseslint from 'typescript-eslint';

// Feature-Sliced Design: each layer may import only from the layers below it.
const LAYERS = ['shared', 'entities', 'features', 'widgets', 'pages', 'app'];

// Slices are consumed only through their public API (index.ts).
const PUBLIC_API_PATTERNS = ['shared', 'entities', 'features', 'widgets', 'pages'].map((layer) => ({
  group: [`@/${layer}/*/*`],
  message: `Импортируйте слайс через его публичный API: "@/${layer}/<slice>".`,
}));

const layerBoundaries = LAYERS.map((layer, index) => {
  const upper = LAYERS.slice(index + 1).map((upperLayer) => ({
    group: [`@/${upperLayer}`, `@/${upperLayer}/*`],
    message: `Слой "${layer}" не может зависеть от вышележащего слоя "${upperLayer}".`,
  }));
  // Features are independent user scenarios and must not know about each other.
  const sameLayer =
    layer === 'features'
      ? [{ group: ['@/features/*'], message: 'Фичи не импортируют друг друга — композиция на уровне widgets/pages.' }]
      : [];

  return {
    files: [`src/${layer}/**/*.{ts,tsx}`],
    rules: {
      'no-restricted-imports': ['error', { patterns: [...upper, ...sameLayer, ...PUBLIC_API_PATTERNS] }],
    },
  };
});

export default tseslint.config(
  { ignores: ['dist', 'coverage', 'public/mockServiceWorker.js'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      '@typescript-eslint/consistent-type-imports': ['error', { fixStyle: 'inline-type-imports' }],
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
    },
  },
  ...layerBoundaries,
  {
    files: ['src/mocks/**/*.ts'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            { group: ['@/features/*', '@/widgets/*', '@/pages/*', '@/app/*'], message: 'Моки зависят только от shared и типов entities.' },
            ...PUBLIC_API_PATTERNS,
          ],
        },
      ],
    },
  },
);
