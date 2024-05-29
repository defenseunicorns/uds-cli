import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';

import CardHeader from './CardHeader.svelte';

describe('CardHeader component', () => {
  const cardHeaderProps = {
    hasLogo: true,
    stacked: false,
    title: 'Mattermost',
    version: 'v1.23.4'
  };

  test('should display the title and version', () => {
    const { getByText } = render(CardHeader, cardHeaderProps);

    expect(getByText(/Mattermost/)).toBeTruthy();
    expect(getByText('v1.23.4')).toBeTruthy();
  });

  test('should stack the logo and the title', () => {
    const { getByTestId } = render(CardHeader, {
      ...cardHeaderProps,
      stacked: true
    });

    expect(getByTestId('card__header-mattermost-test-id').style.flexDirection).toBe('column');
  });
});
