import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';

import HeaderSeparator from './HeaderSeparator.svelte';

describe('HeaderSeparator component', () => {
  test('should display a bar', () => {
    const { container } = render(HeaderSeparator);

    expect(container.querySelector('.custom--header-separator')).toBeDefined();
  });
});
