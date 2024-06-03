import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';

import Header from './Header.svelte';

describe('Header component', () => {
  test('should display a Sign in button when unauthenticated', () => {
    const { getByText } = render(Header, { authenticated: false });

    expect(getByText(/Sign in/)).toBeDefined();
  });

  test('should display prod and account drop downs', () => {
    const { getByTestId, queryByText } = render(Header, { authenticated: true });

    // Make sure we do not see the Sign in button
    expect(queryByText(/Sign in/)).toBeNull();
    expect(getByTestId('bx--header__menu-test-id-username')).toBeDefined();
    expect(getByTestId('bx--header__menu-test-id-username').childElementCount).toBe(3);
  });
});
