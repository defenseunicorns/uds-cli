import { describe, expect, test } from 'vitest';
import { render } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';

import HeaderSelect from './HeaderSelect.svelte';

describe('HeaderSelect component', () => {
  const dropdownOptions = {
    title: 'Services',
    items: [
      {
        title: 'Deployment',
        path: '/deployment'
      },
      {
        title: 'Security',
        path: '/security'
      }
    ]
  };

  test('should display icon withIcon attribute set to true', () => {
    const { getByTestId } = render(HeaderSelect, {
      ...dropdownOptions,
      withIcon: true
    });

    expect(getByTestId('header__select-icon--services-test-id')).toBeTruthy();
  });

  test('should togle menu open on click', async () => {
    const { findByTestId } = render(HeaderSelect, dropdownOptions);

    await userEvent.click(await findByTestId('header__select-menu-action-services-test-id'));
    expect(
      (await findByTestId('header__select-menu-action-services-test-id')).ariaExpanded
    ).toBeTruthy();
  });
});
