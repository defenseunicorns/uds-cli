import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';

import Card from './Card.svelte';

describe('Card component', () => {
  const cardProps = {
    radius: 3,
    width: 303,
    height: 203
  };

  test('should use passed in with, height and border radius', () => {
    const { getByTestId } = render(Card, cardProps);

    expect(getByTestId(/card__container/).style.minWidth).toBe('303px');
    expect(getByTestId(/card__container/).style.minHeight).toBe('203px');
    expect(getByTestId(/card__container/).style.borderRadius).toBe('3px');
  });
});
