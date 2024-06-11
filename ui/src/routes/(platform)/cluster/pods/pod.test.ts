// +page.test.js
import {describe, test} from 'vitest';
import {render, screen} from '@testing-library/svelte';
import Page from './+page.svelte';
import {FakePodRepository} from '../../../../../tests/fakerepos/FakePodRepo';

describe('pod page', () => {
  test('pod rendering', async () => {
    render(Page, {
      data: {
        repo: new FakePodRepository()
      }
    });

    await screen.findByText('pod1');
    await screen.findByText('pod2');
  });
});
