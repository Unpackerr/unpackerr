<script lang="ts">
  import { Modal, ModalHeader, ModalBody } from '@sveltestrap/sveltestrap'
  import Browser from './Index.svelte'
  import { _ } from '../../lib/i18n/Translate.svelte'

  let {
    isOpen = $bindable(false),
    value = $bindable(''),
    title = '',
    description = '',
    disableMkdir = false,
    browseFile = '',
    ...rest
  }: {
    isOpen: boolean
    value: string
    title?: string
    description?: string
    dir?: boolean
    file?: boolean
    disableMkdir?: boolean
    browseFile?: string
  } = $props()

  const close = () => (isOpen = false)
  const heading = $derived(title || $_('FileBrowser.Title'))
  const intro = $derived(
    description ||
      (browseFile ? 'FileBrowser.introFolderFile' : 'FileBrowser.intro'),
  )
</script>

<Modal bind:isOpen size="xl" class="file-browser-modal" toggle={close}>
  <ModalHeader toggle={close}>{heading}</ModalHeader>
  <ModalBody class="p-2">
    <Browser
      bind:value
      height="100%"
      {close}
      description={intro}
      {disableMkdir}
      {browseFile}
      {...rest}
    />
  </ModalBody>
</Modal>
