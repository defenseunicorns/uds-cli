import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';

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
    expect(getByTestId('bx--header__menu-test-id-prod.uds.is')).toBeDefined();
    expect(getByTestId('bx--header__menu-test-id-prod.uds.is').childElementCount).toBe(5);
    expect(getByTestId('bx--header__menu-test-id-username')).toBeDefined();
    expect(getByTestId('bx--header__menu-test-id-username').childElementCount).toBe(3);
  });

  test('should close an open menu when another one is clicked', async () => {
    const { findByTestId } = render(Header, { authenticated: true });

    await userEvent.click(await findByTestId('header__select-menu-action-services-test-id'));
    expect(
      (await findByTestId('header__select-menu-action-services-test-id')).ariaExpanded
    ).toBeTruthy();

    await userEvent.click(await findByTestId('header__select-menu-action-prod.uds.is-test-id'));
    expect((await findByTestId('header__select-menu-action-services-test-id')).ariaExpanded).toBe(
      'false'
    );
  });
});
