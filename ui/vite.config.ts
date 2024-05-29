import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';
import { svelteTesting } from '@testing-library/svelte/vite';

export default defineConfig({
  plugins: [sveltekit(), svelteTesting()],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{js,ts}'],
    setupFiles: ['./vitest-setup.ts'],
    coverage: {
      exclude: [
        '**/src/routes/**',
        '**/tests/**',
        '**/*.config.{js,ts}',
        '**/[.]**',
      ],
    },
  }
});
