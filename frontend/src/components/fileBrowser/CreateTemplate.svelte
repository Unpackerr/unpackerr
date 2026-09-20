<script lang="ts">
  import {
    Button,
    Modal,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import Input from '../Input.svelte'
  import Browser from './Index.svelte'
  import { api } from '../../lib/api'
  import {
    HOOK_TEMPLATE_NAMES,
    defaultHookTemplate,
    hookTemplateFileName,
  } from '../../lib/hooktmpl'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import { untrack } from 'svelte'

  let {
    startPath = '',
    startTemplate = '',
    oncreated,
    onclose,
  }: {
    startPath?: string
    startTemplate?: string
    oncreated: (path: string) => void
    onclose: () => void
  } = $props()

  let isOpen = $state(true)
  let template = $state(untrack(() => defaultHookTemplate(startTemplate)))
  let dest = $state(untrack(() => startPath))
  let error = $state('')
  let saving = $state(false)

  const lockedName = $derived(hookTemplateFileName(template))
  const templateOptions = $derived(
    HOOK_TEMPLATE_NAMES.map((name) => ({ value: name, name })),
  )
  const canSave = $derived(
    !saving &&
      !!dest &&
      (dest.endsWith('/' + lockedName) || dest.endsWith('\\' + lockedName)),
  )

  const close = () => {
    if (saving) return
    isOpen = false
    onclose()
  }

  async function save() {
    if (!canSave) return
    saving = true
    error = ''
    const res = await api.post<{ path?: string }>('browse/template', {
      path: dest,
      template,
    })
    saving = false
    if (!res.ok) {
      error =
        (res.body as { error?: string })?.error || $_('phrases.CreateFailed')
      return
    }
    oncreated(res.body.path || dest)
    isOpen = false
    onclose()
  }
</script>

<Modal bind:isOpen size="xl" class="file-browser-modal" toggle={close}>
  <ModalHeader toggle={close}>{$_('phrases.CreateHookTemplate')}</ModalHeader>
  <ModalBody class="p-2">
    <div class="flex-shrink-0">
      <Input
        id="create-hook-template-name"
        type="select"
        label={$_('config.hooks.template.label')}
        bind:value={template}
        options={templateOptions}
        showChanged={false}
        compact
      />
    </div>
    {#if error}
      <p class="text-danger mb-2 flex-shrink-0">{error}</p>
    {/if}
    <div class="create-template-browser">
      <Browser
        bind:value={dest}
        height="100%"
        close={() => {}}
        description="FileBrowser.introLockedFile"
        dir
        browseFile={lockedName}
        closeOnSelect={false}
        lockName
      />
    </div>
  </ModalBody>
  <ModalFooter>
    <Button color="warning" disabled={saving} onclick={close}>
      {$_('buttons.Cancel')}
    </Button>
    <Button color="success" disabled={!canSave} onclick={save}>
      {#if saving}<Spinner size="sm" />{/if}
      <span class="ms-1">{$_('buttons.Save')}</span>
    </Button>
  </ModalFooter>
</Modal>

<style>
  .create-template-browser {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
</style>
