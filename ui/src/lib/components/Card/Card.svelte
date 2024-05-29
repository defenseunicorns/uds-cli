<script lang="ts">
  export let radius: number = 4;
  export let height: number = 203;
  export let width: number = 297;

  type BorderStyle = {
    borderWidth: number;
    borderColor: string;
  };

  export const borderStyle: BorderStyle = {
    borderWidth: 0,
    borderColor: 'transparent'
  };
</script>

<div
  class="card"
  style:min-height="{height}px"
  style:min-width="{width}px"
  style:border-radius="{radius}px"
  data-testid="card__container"
>
  <slot name="card-header" />
  <slot name="card-content" />
  <slot name="card-links" />
  <slot name="tags" />
</div>

<style lang="scss">
  .card {
    display: grid;
    row-gap: var(--cds-spacing-04);

    background-color: var(--cds-ui-background);
    border: 1px solid var(--cds-link-02);
    border-radius: 4px;
    padding: var(--cds-spacing-05) var(--cds-spacing-06);
  }
</style>
