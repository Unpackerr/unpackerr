<script lang="ts">
  import type { FileBrowser } from './browser.svelte'

  type Props = {
    fb: FileBrowser
    dir: boolean
    dirs: string[]
    files: string[]
    showFiles?: boolean
  }
  const { fb, dir, dirs, files, showFiles = false }: Props = $props()
</script>

<ul class="p-0">
  {#each dirs as folder (folder)}
    <li class="px-2">
      <button type="button" class="file-link" onclick={(e) => fb.cd(e, folder)}
        >{folder}</button
      ><span class="text-muted">{fb.wd.sep}</span>
    </li>
  {/each}
  {#if !dir}
    {#each files as file (file)}
      <li class="px-2">
        <button
          type="button"
          class="file-link"
          onclick={(e) => fb.select(e, file)}>{file}</button
        >
      </li>
    {/each}
  {:else if showFiles}
    {#each files as file (file)}
      <li class="px-2 text-muted">{file}</li>
    {/each}
  {/if}
</ul>

<style>
  .file-link {
    background: none;
    border: none;
    padding: 0;
    color: var(--bs-link-color);
    text-decoration: none;
    cursor: pointer;
    font: inherit;
    text-align: left;
  }

  .file-link:hover {
    color: var(--bs-link-hover-color);
    text-decoration: underline;
  }

  ul {
    columns: auto 200px;
    column-gap: 0;
    list-style: none;
  }

  ul li {
    break-inside: avoid-column;
  }

  ul li:nth-child(even) {
    background-color: var(--bs-card-cap-bg);
  }

  ul li:hover {
    background-color: var(--bs-tertiary-bg);
  }
</style>
