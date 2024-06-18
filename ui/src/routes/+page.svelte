<svelte:head>
  <title>Home</title>
  <meta name="description" content="Home -- Search for repos" />
</svelte:head>

<div style:margin-left="2em" style:margin-top="4em">
  <div class="welcome__container">
    <h2 class="header">Welcome to UDS</h2>

    <p class="subheader">
      Find the applications running in your cluster and accessible to you here.
    </p>

    <div class="cards">
      <h2>cards used to go here</h2>
    </div>
  </div>
</div>

<style lang="scss">
  .welcome__container {
    display: grid;
    grid-gap: 17px;
  }

  .header {
    font-size: 16px;
    font-weight: 600;
    // line-height: 32px;
  }

  .subheader {
    color: var(--cds-text-secondary);
    font-size: var(--cds-helper-text-01-font-size);
    line-height: var(--cds-helper-text-01-font-size);
    letter-spacing: 0.32px;
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    grid-gap: var(--cds-spacing-05);
  }
</style>
