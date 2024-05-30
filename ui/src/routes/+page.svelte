<script lang="ts">
  import Card from '$lib/components/Card/Card.svelte';
  import CardHeader from '$lib/components/Card/CardHeader.svelte';
  import CardContent from '$lib/components/Card/CardContent.svelte';
  import CardLinks from '$lib/components/Card/CardLinks.svelte';
  import { type TagType } from '$lib/components/Shared/Tag/types';

  const tags: TagType[] = [
    {
      name: 'Category',
      type: 'green'
    },
    {
      name: 'Secondary',
      type: 'purple'
    }
  ];
</script>

<svelte:head>
  <title>Home</title>
  <meta name="description" content="Home -- Search for repos" />
</svelte:head>

<div>
  <h2 class="header">Welcome to UDS</h2>

  <p class="subheader">Find the applications running in your cluster and accessible to you here.</p>

  <div class="cards">
    <Card radius={4} width={300} height={200}>
      <CardHeader slot="card-header" hasLogo stacked={false} version="v1.23.4" title="App Name" />

      <CardContent
        slot="card-content"
        text="App Description. Your son is gone. He was weak and foolish like his father."
      />

      <CardLinks
        slot="card-links"
        links={[
          {
            name: 'Link',
            url: 'http://www.google.com'
          },
          {
            name: 'Link',
            url: 'http://www.google.com'
          }
        ]}
      />
    </Card>

    <Card radius={4} width={300} height={200}>
      <CardHeader slot="card-header" hasLogo stacked={false} version="v1.23.4" title="Gitlab" />

      <CardContent
        slot="card-content"
        text="App Description. Your son is gone. He was weak and foolish like his father."
      />

      <CardLinks
        slot="card-links"
        links={[
          {
            name: 'Link',
            url: 'http://www.google.com'
          },
          {
            name: 'Link',
            url: 'http://www.google.com'
          }
        ]}
      />
    </Card>

    <Card radius={4} width={300} height={200}>
      <CardHeader slot="card-header" hasLogo stacked={false} version="v1.23.4" title="Mattermost" />

      <CardContent
        slot="card-content"
        text="App Description. Your son is gone. He was weak and foolish like his father."
      />

      <CardLinks
        slot="card-links"
        links={[
          {
            name: 'Link',
            url: 'http://www.google.com'
          },
          {
            name: 'Link',
            url: 'http://www.google.com'
          }
        ]}
      />
    </Card>

    <Card radius={4} width={300} height={200}>
      <CardHeader
        slot="card-header"
        hasLogo
        stacked={false}
        version="v1.23.4"
        title="Pipeline Tool"
      />

      <CardContent
        slot="card-content"
        text="App Description. Your son is gone. He was weak and foolish like his father."
      />

      <CardLinks
        slot="card-links"
        links={[
          {
            name: 'Link',
            url: 'http://www.google.com'
          },
          {
            name: 'Link',
            url: 'http://www.google.com'
          }
        ]}
      />
    </Card>

    <Card radius={4} width={300} height={200}>
      <CardHeader slot="card-header" hasLogo stacked={false} version="v1.23.4" title="LeapfrogAi" />

      <CardContent
        slot="card-content"
        text="App Description. Your son is gone. He was weak and foolish like his father."
      />

      <CardLinks
        slot="card-links"
        links={[
          {
            name: 'uds.ai/char',
            url: 'http://www.google.com'
          }
        ]}
      />
    </Card>
  </div>
</div>

<style lang="scss">
  .header {
    font-size: 16px;
    font-weight: 600;
    line-height: 32px;
  }

  .subheader {
    font-size: var(--cds-helper-text-01-font-size);
    line-height: 16px;
    letter-spacing: 0.32px;
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    grid-column-gap: var(--cds-spacing-04);
    grid-row-gap: var(--cds-spacing-06);

    margin-top: var(--cds-spacing-07);
  }
</style>
