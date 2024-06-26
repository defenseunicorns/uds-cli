import { render } from '@testing-library/svelte';
import { vi, describe, test, expect } from 'vitest';

import Page from './+page.svelte';
import { FakePolicyLogsRepo } from '../../../../../tests/fakerepos/FakePolicyRepo';

describe('Monitor/Pepr page', () => {
  test('should render data', async () => {
    const { findAllByText } = render(Page, {
      data: {
        policyLogsRepo: FakePolicyLogsRepo,
        url: 'fake-url'
      }
    });

    // stub out scrollIntoView because it doesn't exist in jsdom
    Element.prototype.scrollIntoView = vi.fn();

    const allowedText = await findAllByText('ALLOWED', { exact: false });

    expect(allowedText.length).toBeGreaterThan(0);
  });
});
