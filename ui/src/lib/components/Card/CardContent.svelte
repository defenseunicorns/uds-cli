<script lang="ts">
  export let text: string = '';
</script>

<slot name="card-content">
  <section class="card__content">
    {text}
  </section>
</slot>

<style lang="scss">
  .card__content {
    font-size: 12px;
  }
</style>
