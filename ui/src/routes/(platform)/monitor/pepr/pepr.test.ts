import {waitFor, render, findByText} from '@testing-library/svelte';
import {vi, describe, test, expect} from 'vitest';
import Page from './+page.svelte';
import {FakePolicyLogsRepo} from "../../../../../tests/fakerepos/FakePolicyRepo";

describe('monitor pepr page', () => {
    test('data is rendered', async () => {

        const {findByText} = render(Page, {
            data: {
                policyLogsRepo: new FakePolicyLogsRepo()
            }
        });

        // stub out scrollIntoView because it doesn't exist in jsdom
        Element.prototype.scrollIntoView = vi.fn()

        expect(await findByText('VALIDATE', {exact: false})).toBeDefined();
    });
});
