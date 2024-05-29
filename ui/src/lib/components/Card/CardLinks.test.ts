import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';

import CardLinks from './CardLinks.svelte';
import { type LinkType } from './types';

describe('CardLinks component', () => {
  const cardLinksProps: LinkType[] =[
    {
      name: 'Link 1',
      url: 'http://www.google.com'
    },
    {
      name: 'Link 2',
      url: 'http://www.google.com'
    }
  ];

  test('should display links', () => {
    const { getByText } = render(CardLinks, { links: cardLinksProps });

    expect(getByText(/Link 1/)).toBeTruthy();
  });
});
