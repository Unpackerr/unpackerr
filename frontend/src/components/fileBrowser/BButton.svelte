<script lang="ts">
  import { Button } from '@sveltestrap/sveltestrap'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import { theme } from '../../lib/theme.svelte'

  type Props = { onclick?: () => void; isOpen?: boolean }
  let { onclick, isOpen = $bindable(false) }: Props = $props()
  const click = () => (onclick?.(), (isOpen = true))
  const color = $derived(theme.resolved === 'dark' ? 'Moccasin' : 'Gold')
</script>

<Button
  type="button"
  class="btn-icon"
  outline
  onclick={click}
  title={$_('phrases.BrowseFiles')}
  aria-label={$_('phrases.BrowseFiles')}
>
  <!-- phosphor FolderOpen, inlined so we do not add a new deep import -->
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="1.25em"
    height="1.25em"
    fill={color}
    viewBox="0 0 256 256"
    aria-hidden="true"
  >
    <path
      d="M245,110.64l-21.83,82.63A16,16,0,0,1,207.69,204H48.31a16,16,0,0,1-15.43-11.73L10.85,109.64A8,8,0,0,1,18.56,100H88L70.22,59.56A8,8,0,0,1,77.78,48H208a8,8,0,0,1,0,16H86.94l14.4,32H241.44A8,8,0,0,1,245,110.64Z"
    ></path>
  </svg>
</Button>
