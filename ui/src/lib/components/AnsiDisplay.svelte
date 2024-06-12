<script lang="ts">
  import { onMount } from 'svelte';
  import Convert from 'ansi-to-html';

  const convert = new Convert({
    newline: true,
    stream: true,
    colors: {
      0: '#000000',
      1: '#C23621',
      2: '#25BC24',
      3: '#ADAD27',
      4: '#000080',
      5: '#D338D3',
      6: '#33BBC8',
      7: '#CBCCCD',
      8: '#818383',
      9: '#FC391F',
      10: '#31E783',
      11: '#EAEC23',
      12: '#0000E1',
      13: '#F935F8',
      14: '#14F0F0',
      15: '#E9EBEB'
    }
  });
  let termElement: HTMLElement | null;
  let scrollAnchor: Element | null | undefined;

  export let height = '688px';
  export let width = 'auto';
  export let minWidth = '';
  export let maxWidth = '';

  // exported in parent component to handle incoming SSE messages
  export const addMessage = (message: string) => {
    let html = convert.toHtml(message);
    html = `<div class="zarf-terminal-line">${html}</div>`;
    scrollAnchor?.insertAdjacentHTML('beforebegin', html);
    scrollAnchor?.scrollIntoView();
  };

  onMount(() => {
    termElement = document.getElementById('terminal');
    scrollAnchor = termElement?.lastElementChild;
  });
</script>

<div id="terminal">
  <div class="scroll-anchor" />
</div>

<style>
  #terminal {
    display: flex;
    flex-direction: column;
    background-color: #1e1e1e;
    padding: 12px;
    font-size: 12px;
    overflow-x: auto;
    overflow-y: hidden;

    height: var(--height);
    width: 90%;
    min-width: var(--minWidth);
    max-width: var(--maxWidth);
  }

  & .zarf-terminal-line {
    white-space: pre-wrap;
    word-break: break-all;
    word-wrap: break-word;
    overflow-wrap: break-word;
  }
</style>
