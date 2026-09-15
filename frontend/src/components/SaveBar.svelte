<script lang="ts">
  import type { Snippet } from 'svelte'
  import golift from '../assets/golift.png'

  let { children }: { children: Snippet } = $props()
</script>

<div class="save-bar-spacer" aria-hidden="true"></div>
<div class="save-bar">
  <div class="container-xxl page-wrap save-bar-inner">
    <div class="save-bar-actions">
      {@render children()}
    </div>
    <a
      class="save-bar-brand"
      href="https://golift.io"
      target="_blank"
      rel="noopener noreferrer"
      title="Go Lift"
    >
      <img src={golift} alt="Go Lift" />
    </a>
  </div>
</div>
