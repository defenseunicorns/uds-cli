import flowbitePlugin from 'flowbite/plugin';

import type { Config } from 'tailwindcss';

export default {
  content: [
    './src/**/*.{html,js,svelte,ts}',
    './node_modules/flowbite/**/*.js',
    './node_modules/flowbite-svelte-icons/**/*.{html,js,svelte,ts}'
  ],

  theme: {
    fontFamily: {
      body: [
        'Montserrat',
        'ui-sans-serif',
        'system-ui',
        '-apple-system',
        'system-ui',
        'Segoe UI',
        'Roboto',
        'Helvetica Neue',
        'Arial',
        'Noto Sans',
        'sans-serif',
        'Apple Color Emoji',
        'Segoe UI Emoji',
        'Segoe UI Symbol',
        'Noto Color Emoji'
      ],
      sans: [
        'Montserrat',
        'ui-sans-serif',
        'system-ui',
        '-apple-system',
        'system-ui',
        'Segoe UI',
        'Roboto',
        'Helvetica Neue',
        'Arial',
        'Noto Sans',
        'sans-serif',
        'Apple Color Emoji',
        'Segoe UI Emoji',
        'Segoe UI Symbol',
        'Noto Color Emoji'
      ]
    }
  },

  variants: {},

  plugins: [flowbitePlugin]
} as Config;
