<script lang="ts">
  import {
    InputGroup,
    InputGroupText,
    Input,
    Modal,
    ModalBody,
    Button,
    ModalHeader,
    Card,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import Icon from '../Icon.svelte'
  import T, { _ } from '../../lib/i18n/Translate.svelte'
  import type { FileBrowser } from './browser.svelte'
  import type { Snippet } from 'svelte'
  import { slide } from 'svelte/transition'
  import { has } from '../../lib/auth.svelte'
  import { systemPerm } from '../../lib/perms'

  type Props = {
    filter: string
    fb: FileBrowser
    disableMkdir?: boolean
    children?: Snippet
  }
  let {
    filter = $bindable(),
    fb,
    disableMkdir = false,
    children,
  }: Props = $props()
  let newPath = $state('')
  let isOpen = $state(false)
  let showTooltip = $state(false)
  const canMkdir = $derived(!disableMkdir && has(systemPerm('browse', 'write')))

  const cancel = () => ((newPath = ''), (isOpen = false))
  const create = async (e: Event, name: string) => {
    e.preventDefault()
    if (await fb.mkdir(name)) {
      isOpen = false
      newPath = ''
    }
  }
</script>

<InputGroup class="my-2">
  <Button
    color="secondary"
    class="btn-icon"
    onclick={() => (showTooltip = !showTooltip)}
    outline
    title={$_('phrases.ShowMore')}
  >
    <Icon help />
  </Button>

  <InputGroupText><T id="FileBrowser.FilterFiles" /></InputGroupText>
  <Input bind:value={filter} />

  {#if canMkdir && fb.wd.path}
    <Button
      color="warning"
      outline
      type="button"
      onclick={() => (isOpen = true)}
      disabled={!fb.wd.path}
    >
      <T id="buttons.CreateFolder" />
    </Button>
  {/if}
</InputGroup>

{#if showTooltip}
  <div transition:slide>
    <Card body class="mt-1" color="warning" outline>
      <p class="mb-0">{@html $_('FileBrowser.FilterFilesDesc')}</p>
    </Card>
  </div>
{:else}
  <div transition:slide>{@render children?.()}</div>
{/if}

{#if canMkdir}
  <Modal bind:isOpen contentClassName="border-warning-subtle">
    <ModalHeader toggle={cancel}><T id="buttons.CreateFolder" /></ModalHeader>
    <ModalBody>
      <T id="FileBrowser.CreateFolderIn" path={fb.wd.path} />
      <form onsubmit={(e) => create(e, newPath)}>
        <InputGroup>
          <Input bind:value={newPath} />
          <Button
            color="success"
            outline
            type="submit"
            disabled={!newPath || fb.loading}
          >
            {#if fb.loading}
              <Spinner size="sm" />
              <T id="FileBrowser.Creating" />
            {:else}
              <T id="buttons.Create" />
            {/if}
          </Button>
        </InputGroup>
      </form>
    </ModalBody>
  </Modal>
{/if}
