import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';

import CardContent from './CardContent.svelte';

describe('CardContent component', () => {
  const cardContentProps = {
    text: 'I am a test'
  };

  test('should display content text', () => {
    const { getByText } = render(CardContent, cardContentProps);

    expect(getByText(/I am a test/)).toBeTruthy();
  });
});
