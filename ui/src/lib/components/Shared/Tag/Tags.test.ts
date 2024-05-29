import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';

import Tags from './Tags.svelte';
import { type TagType } from '$lib/components/Shared/Tag/types';

describe('Tags component', () => {
  const tags: TagType[] = [
    {
      name: 'Category',
      type: 'green'
    },
    {
      name: 'Secondary',
      type: 'purple'
    }
  ];

  test('should display Tags', () => {
    const { getByText } = render(Tags, {tags});

    expect(getByText(/Category/)).toBeTruthy();
  });
});
