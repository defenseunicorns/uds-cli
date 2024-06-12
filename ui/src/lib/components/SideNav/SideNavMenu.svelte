<script lang="ts">
  export let text: string;
</script>

<li class="bx--side-nav__item">
  <div aria-expanded="true" class="bx--side-nav__submenu">
    <span class="bx--side-nav__submenu-title">{text}</span>
  </div>

  {#if $$slots.default}
    <ul role="menu" class="bx--side-nav__menu">
      <slot />
    </ul>
  {/if}
</li>
