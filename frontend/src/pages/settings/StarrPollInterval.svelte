<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Card, CardBody, Col, Row } from '@sveltestrap/sveltestrap'
  import Input from '../../components/Input.svelte'
  import { has } from '../../lib/auth.svelte'
  import { configPerm } from '../../lib/perms'
  import { loadSection } from '../../lib/config'
  import { trackDirty } from '../../lib/dirty.svelte'
  import { deepCopy } from '../../lib/util'
  import type { GeneralConfig } from '../../lib/types'
  import {
    clearStarrPoll,
    starrPoll,
    starrPollDirty,
    starrPollRetains,
  } from './starr-poll.svelte'

  const canRead = $derived(has(configPerm('general', 'read')))
  const canWrite = $derived(has(configPerm('general', 'write')))

  trackDirty(() => starrPollDirty(), starrPollRetains)

  onMount(async () => {
    if (!canRead) return
    const { data } = await loadSection<GeneralConfig>('general')
    if (data) {
      starrPoll.general = data
      starrPoll.orig = deepCopy(data)
    }
  })

  onDestroy(clearStarrPoll)
</script>

{#if starrPoll.general}
  <Card class="mb-3">
    <CardBody>
      <Row class="g-2">
        <Col md="6">
          <Input
            id="config.general.interval"
            type="interval"
            bind:value={starrPoll.general.interval}
            original={starrPoll.orig?.interval}
            disabled={!canWrite}
            envVar="INTERVAL"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.logQueues"
            type="logqueue"
            bind:value={starrPoll.general.logQueues}
            original={starrPoll.orig?.logQueues}
            disabled={!canWrite}
            envVar="LOG_QUEUES"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.activity"
            type="select"
            bind:value={starrPoll.general.activity}
            original={starrPoll.orig?.activity}
            disabled={!canWrite}
            envVar="ACTIVITY"
          />
        </Col>
      </Row>
    </CardBody>
  </Card>
{/if}
