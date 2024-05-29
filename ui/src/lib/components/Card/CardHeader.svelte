<script lang="ts">
  import { truncate } from "carbon-components-svelte";

  export let hasLogo: boolean = true;
  export let stacked: boolean = false;
  export let title: string = '';
  export let version: string = '';
</script>

<slot name="card-header">
  <section
    class="card__header"
    style:flex-direction={stacked ? 'column' : 'row'}
    style:align-items={stacked ? 'flex-start' : 'center'}
    use:truncate
    data-testid="card__header-{title.toLocaleLowerCase()}-test-id"
  >
    {#if hasLogo}
      <div class="card__header-logo" style:margin-bottom={stacked ? 'var(--cds-spacing-03)' : 0}>
        <img
          alt="Defense Unicorns Logo"
          src="https://www.defenseunicorns.com/images/svg/doug.svg"
          style="width: 40px; height: 40px"
        />
      </div>
    {/if}

    <div class="card__header-info" use:truncate>
      <div class="card__header-title">{title}</div>
      <div class="card__header-version" use:truncate>{version}</div>
    </div>
  </section>
</slot>

<style lang="scss">
  .card__header {
    display: flex;
    flex-direction: row;
    align-items: center;
    font-size: var(--cds-body-01-font-size);

    &-logo {
      margin-right: var(--cds-spacing-03);
    }

    &-info {
      display: flex;
      flex-direction: row;
    }

    &-title {
      font-weight: var(--cds-display-02-font-weight);
      margin-right: var(--cds-spacing-03);
    }
  }
</style>
