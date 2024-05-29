<script lang="ts">
  export let spaceLeft: number = 0;
  export let spaceRight: number = 0;
</script>

<div
  class="custom--header-separator"
  style="margin-left: {spaceLeft}px; margin-right: {spaceRight}px"
/>

<style lang="scss">
  .custom--header-separator {
    display: flex;
    align-self: center;
    height: 50%;
    width: 1px;
    background-color: var(--cds-ui-01);
  }
</style>
