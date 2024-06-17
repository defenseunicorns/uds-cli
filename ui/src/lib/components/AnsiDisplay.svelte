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

  // using Zarf UI constant for now: https://github.com/defenseunicorns/zarf-ui/blob/d02a5c0e4e04441d6bb3bd7ed331e037a35aa067/src/ui/lib/components/ansi-display.svelte#L35C27-L35C29
  export let height = '688px';

  // exported in parent component to handle incoming SSE messages
  export const addMessage = (message: string) => {
    let html = convert.toHtml(message);
    html = `<div class="zarf-terminal-line">${html}</div>`;
    console.log('scrollAnchor', scrollAnchor)
    scrollAnchor?.insertAdjacentHTML('beforebegin', html);
    scrollAnchor?.scrollIntoView();
  };

  onMount(() => {
    termElement = document.getElementById('terminal');
    scrollAnchor = termElement?.lastElementChild;
  });
</script>

<div id="terminal" style="--box-height: {height}">
  <div class="scroll-anchor" />
</div>

<style>
  #terminal {
    display: flex;
    flex-direction: column;
    padding: 12px;
    font-size: 12px;
    overflow-x: auto;
    overflow-y: auto;
    height: var(--box-height);
    width: 100%;
  }

  /* dynamically rendered terminal lines */
  .zarf-terminal-line {
    white-space: pre-wrap;
    word-break: break-all;
    word-wrap: break-word;
    overflow-wrap: break-word;
    width: 100%;
  }
</style>
