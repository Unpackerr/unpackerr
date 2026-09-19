<script lang="ts">
  import {
    Button,
    Card,
    CardBody,
    CardFooter,
    CardHeader,
    InputGroup,
    InputGroupText,
    Input,
    Tooltip,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import T from '../../lib/i18n/Translate.svelte'
  import { type Snippet, untrack } from 'svelte'
  import FileList from './FileList.svelte'
  import { FileBrowser } from './browser.svelte'
  import ActionBar from './ActionBar.svelte'
  import { slide } from 'svelte/transition'
  import { theme } from '../../lib/theme.svelte'

  type Props = {
    value: string
    description?: string
    close: () => void
    dir?: boolean
    file?: boolean
    disableMkdir?: boolean
    browseFile?: string
    height?: string
    children?: Snippet
    footer?: Snippet
  }

  let {
    value = $bindable(),
    close,
    description = 'FileBrowser.intro',
    dir = false,
    file = false,
    disableMkdir = false,
    browseFile = '',
    height = '100%',
    children,
    footer,
  }: Props = $props()

  const uid = $props.id()
  let filter = $state('')
  const fb = new FileBrowser(
    untrack(() => value),
    (v) => ((value = v), close()),
    untrack(() => browseFile),
  )
  const filt = $derived(filter.toLowerCase())
  const dirs = $derived(
    fb.wd.dirs?.filter((d) => d.toLowerCase().includes(filt)) || [],
  )
  const files = $derived(
    fb.wd.files?.filter((f) => f.toLowerCase().includes(filt)) || [],
  )
  const dirsCount = $derived(fb.wd.dirs?.length ?? 0)
  const fileCount = $derived(fb.wd.files?.length ?? 0)
</script>

<div class="file-browser">
  <Card style="height: {height};min-height: 400px;">
    <CardHeader>
      <form onsubmit={(e) => fb.cd(e, fb.input, true)}>
        <InputGroup>
          <Button
            id="{uid}-up"
            class="btn-icon"
            outline
            onclick={(e) => fb.cd(e, fb.wd.mom || fb.wd.sep, true)}
            disabled={fb.loading}
            type="button"
          >
            {#if fb.loading}
              <Spinner size="sm" />
            {:else}
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="1.25em"
                height="1.25em"
                fill="currentColor"
                viewBox="0 0 256 256"
                aria-hidden="true"
              >
                <path
                  d="M224,160v40a16,16,0,0,1-16,16H48a16,16,0,0,1-16-16V160a8,8,0,0,1,16,0v40H208V160a8,8,0,0,1,16,0ZM85.66,77.66,120,43.31V144a8,8,0,0,0,16,0V43.31l34.34,34.35a8,8,0,0,0,11.32-11.32l-48-48a8,8,0,0,0-11.32,0l-48,48A8,8,0,0,0,85.66,77.66Z"
                ></path>
              </svg>
            {/if}
          </Button>
          <Tooltip target="{uid}-up" theme={theme.tooltip}>
            <T id="FileBrowser.tooltip.Up" />
          </Tooltip>
          <InputGroupText><T id="FileBrowser.Path" /></InputGroupText>
          <Input bind:value={fb.input} />
          {#if fb.input !== fb.wd.path}
            <Button id="{uid}-go" type="submit" color="primary" outline>
              <T id="buttons.Go" />
            </Button>
            <Tooltip target="{uid}-go" theme={theme.tooltip}>
              <T id="FileBrowser.tooltip.Go" path={fb.input} />
            </Tooltip>
          {/if}

          {#if !file || fb.input !== fb.wd.path}
            <Button
              id="{uid}-select"
              class="btn-icon"
              color="success"
              outline
              type="button"
              onclick={(e) => fb.select(e, fb.input, true)}
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="1.25em"
                height="1.25em"
                fill="limegreen"
                viewBox="0 0 256 256"
                aria-hidden="true"
              >
                <path
                  d="M229.66,77.66l-128,128a8,8,0,0,1-11.32,0l-56-56a8,8,0,0,1,11.32-11.32L96,188.69,218.34,66.34a8,8,0,0,1,11.32,11.32Z"
                ></path>
              </svg>
            </Button>
            <Tooltip target="{uid}-select" theme={theme.tooltip}>
              <T
                id="FileBrowser.tooltip.SelectPath"
                path={fb.preview(fb.input)}
              />
            </Tooltip>
          {/if}
        </InputGroup>
      </form>
      <T id={description} file={fb.fileName} />
      <ActionBar bind:filter {fb} {disableMkdir}>
        {@render children?.()}
      </ActionBar>
    </CardHeader>

    <CardBody class="overflow-auto h-100 p-0">
      {#if fb.respErr}
        <div transition:slide>
          <Card outline color="danger" class="m-2 text-center" body
            >{fb.respErr}</Card
          >
        </div>
      {/if}
      <FileList {fb} {dir} {dirs} {files} showFiles={!!browseFile} />
    </CardBody>

    <CardFooter>
      <ul class="d-inline-block mb-0 ps-2">
        {#if !filt}
          <li><T id="FileBrowser.Folders" count={dirsCount} /></li>
          <li><T id="FileBrowser.Files" count={fileCount} /></li>
        {:else}
          <li>
            <T
              id="FileBrowser.FoldersFiltered"
              count={dirsCount}
              filtered={dirs.length}
            />
          </li>
          <li>
            <T
              id="FileBrowser.FilesFiltered"
              count={fileCount}
              filtered={files.length}
            />
          </li>
        {/if}
        {#if value}<li><T id="FileBrowser.Selected" path={value} /></li>{/if}
      </ul>
      {@render footer?.()}
    </CardFooter>
  </Card>
</div>

<style>
  .file-browser :global(.input-group > .btn-icon) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    padding-left: 0;
    padding-right: 0;
  }
</style>
