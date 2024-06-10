// +page.test.js
import {describe, expect, test} from 'vitest';
import {render, waitFor} from '@testing-library/svelte';
import Page from './+page.svelte';
import {FakePodRepository} from "../../../../../tests/fakerepo";

describe('pod page', () => {
    test('pod rendering', async () => {
        const {getByText} = render(Page, {
            data: {
                repo: new FakePodRepository()
            }
        });

        await waitFor(() => expect(getByText('pod1')).toBeDefined());
        expect(getByText('pod2')).toBeDefined();
    });
});
