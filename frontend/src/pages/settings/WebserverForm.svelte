<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Alert,
    Badge,
    Button,
    Card,
    CardBody,
    CardHeader,
    Col,
    FormCheck,
    FormGroup,
    Label,
    Row,
    Spinner,
    Collapse,
  } from '@sveltestrap/sveltestrap'
  import Input from '../../components/Input.svelte'
  import SaveBar from '../../components/SaveBar.svelte'
  import PageIntro from '../../components/PageIntro.svelte'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import { loadSection, loadSectionLive, saveSection } from '../../lib/config'
  import { has, profile, restore } from '../../lib/auth.svelte'
  import {
    ALL_PERMISSIONS,
    RoleAdmin,
    configPerm,
    newApiKey,
  } from '../../lib/perms'
  import { roleNameDuplicateError, roleNameError } from '../../lib/validate'
  import { deepCopy, deepEqual } from '../../lib/util'
  import { trackDirty } from '../../lib/dirty.svelte'
  import type { WebServer, APIKey } from '../../lib/types'
  import { envHas, loadEnv } from '../../lib/env.svelte'
  import { deriveKDF } from '../../lib/kdf'
  import {
    type AuthMode,
    defaultAuthHeader,
    defaultAuthRoleHeader,
    headerPickerOptions,
    minUIPassword,
    parseStoredPassword,
    reservedUsername,
  } from '../../lib/ui-auth'

  let { pane = 'server' }: { pane?: 'server' | 'auth' } = $props()

  let cfg = $state<WebServer | null>(null)
  let orig = $state<WebServer | null>(null)
  let upstreamsText = $state('')
  let origUpstreams = $state('')
  let wsOriginsText = $state('')
  let origWSOrigins = $state('')
  let roleList = $state<{ name: string; permissions: string[] }[]>([])
  let origRoles = $state<Record<string, { permissions: string[] }> | null>(null)
  let origRoleList = $state<{ name: string; permissions: string[] }[]>([])
  let permOpen = $state<boolean[]>([])
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let authType = $state<AuthMode>('password')
  let origAuthType = $state<AuthMode>('password')
  let username = $state('admin')
  let origUsername = $state('admin')
  let headerName = $state(defaultAuthHeader)
  let origHeader = $state(defaultAuthHeader)
  let currentPass = $state('')
  let newPass = $state('')

  const canWrite = has(configPerm('webserver', 'write'))
  const authEnv = $derived(envHas('WEBSERVER_UI_PASSWORD'))
  const upstreamAllowed = $derived(!!profile.info?.upstreamAllowed)
  const filepathPass = $derived(!!orig?.uiPassword?.startsWith('filepath:'))
  const logFileCountChoices = $derived(
    [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 30, 40, 50].map((n) => ({
      value: n,
      name: n === 0 ? $_('words.select-option.NoRotation') : String(n),
    })),
  )
  const logFileMbChoices = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20].map((n) => ({
    value: n,
    name: String(n),
  }))

  onMount(async () => {
    await loadEnv()

    const { data, error: err } = await loadSection<WebServer>('webserver')
    if (err) {
      error = err
    } else {
      cfg = data!
      orig = deepCopy(data!)
      upstreamsText = (cfg.upstreams ?? []).join('\n')
      origUpstreams = upstreamsText
      wsOriginsText = (cfg.wsOrigins ?? []).join('\n')
      origWSOrigins = wsOriginsText
      roleList = Object.entries(cfg.roles ?? {}).map(([name, r]) => ({
        name,
        permissions: [...(r.permissions ?? [])],
      }))
      origRoles = deepCopy(cfg.roles)
      origRoleList = deepCopy(roleList)
      permOpen = roleList.map(() => false)
      cfg.uiRoleHeader ??= ''
      orig.uiRoleHeader ??= ''
      applyAuth(cfg.uiPassword, true)
      await applyLiveAuth()
    }

    loading = false
  })

  function applyAuth(raw: string, asOrig: boolean) {
    const parsed = parseStoredPassword(raw)
    authType = parsed.mode
    username = parsed.username
    headerName = parsed.header

    if (asOrig) {
      origAuthType = parsed.mode
      origUsername = parsed.username
      origHeader = parsed.header
    }
  }

  async function applyLiveAuth() {
    const envPass = envHas('WEBSERVER_UI_PASSWORD')
    const filePass = orig?.uiPassword?.startsWith('filepath:')
    if (!envPass && !filePass) return

    const { data } = await loadSectionLive<WebServer>('webserver')
    if (data?.uiPassword) applyAuth(data.uiPassword, true)
  }

  const loginDirty = $derived(
    authType !== origAuthType ||
      (authType === 'password' && username !== origUsername) ||
      ((authType === 'header' || authType === 'noauth') &&
        headerName !== origHeader) ||
      !!newPass,
  )

  const reservedUser = $derived(reservedUsername(username))
  const needCurrent = $derived(
    origAuthType === 'password' && loginDirty && !authEnv,
  )
  const headerOptions = $derived(headerPickerOptions(profile.info?.headers))
  const roleHeaderOptions = $derived.by(() => {
    const none = {
      value: '',
      name: $_('config.webserver.roleHeader.options.none'),
    }
    const extra = headerOptions.some((o) => o.value === defaultAuthRoleHeader)
      ? []
      : [{ value: defaultAuthRoleHeader, name: defaultAuthRoleHeader }]
    return [none, ...extra, ...headerOptions]
  })
  const authInvalid = $derived(
    (authType === 'password' && (!username.trim() || reservedUser)) ||
      (authType === 'password' &&
        newPass.length > 0 &&
        newPass.length < minUIPassword) ||
      (authType === 'password' &&
        origAuthType !== 'password' &&
        newPass.length < minUIPassword) ||
      (authType === 'password' && username !== origUsername && !newPass) ||
      (authType === 'header' && !headerName.trim()) ||
      (needCurrent && currentPass.length < minUIPassword),
  )

  trackDirty(
    () =>
      !loading &&
      cfg != null &&
      orig != null &&
      (!deepEqual(cfg, orig) ||
        upstreamsText !== origUpstreams ||
        wsOriginsText !== origWSOrigins ||
        !deepEqual(roleList, origRoleList) ||
        loginDirty),
  )

  const roleNames = $derived.by(() => {
    const names = [RoleAdmin]
    const seen = new Set([RoleAdmin])
    for (const r of roleList) {
      const n = r.name.trim()
      if (!n || seen.has(n)) continue
      seen.add(n)
      names.push(n)
    }
    return names
  })
  const rolesEnv = $derived(envHas('WEBSERVER_ROLES_*'))
  const rolesInvalid = $derived(
    roleList.some(
      (r, i) =>
        !!roleNameError(r.name) ||
        !!roleNameDuplicateError(r.name, otherRoleNames(i)),
    ),
  )

  function otherRoleNames(i: number): string[] {
    return roleList.filter((_, idx) => idx !== i).map((r) => r.name)
  }

  function addKey() {
    if (cfg)
      cfg.apiKeys = [
        ...cfg.apiKeys,
        { name: '', key: newApiKey(), roles: [RoleAdmin] },
      ]
  }

  function removeKey(i: number) {
    if (cfg) cfg.apiKeys = cfg.apiKeys.filter((_, idx) => idx !== i)
  }

  function toggleKeyRole(key: APIKey, role: string, on: boolean) {
    key.roles = on
      ? [...new Set([...key.roles, role])]
      : key.roles.filter((r) => r !== role)
  }

  function addRole() {
    roleList = [...roleList, { name: '', permissions: [] }]
    permOpen = [...permOpen, true]
  }

  function removeRole(i: number) {
    roleList = roleList.filter((_, idx) => idx !== i)
    permOpen = permOpen.filter((_, idx) => idx !== i)
  }

  function togglePermOpen(i: number) {
    permOpen[i] = !permOpen[i]
  }

  function toggleRolePerm(
    role: { permissions: string[] },
    perm: string,
    on: boolean,
  ) {
    role.permissions = on
      ? [...new Set([...role.permissions, perm])]
      : role.permissions.filter((p) => p !== perm)
  }

  function addThisIP() {
    const ip = profile.info?.clientIP?.trim()
    if (!ip) return
    const lines = upstreamsText
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
    if (
      lines.includes(ip) ||
      lines.includes(ip + '/32') ||
      lines.includes(ip + '/128')
    )
      return
    upstreamsText = lines.concat(ip).join('\n')
  }

  async function storedPassword(): Promise<string> {
    if (authType === 'header')
      return 'webauth:' + (headerName.trim() || defaultAuthHeader)
    if (authType === 'noauth')
      return headerName.trim() ? 'noauth:' + headerName.trim() : 'noauth'
    if (newPass) {
      const name = username.trim() || 'admin'
      return name + ':' + (await deriveKDF(name, newPass))
    }
    return cfg?.uiPassword ?? ''
  }

  async function save() {
    if (!cfg || authInvalid || rolesInvalid) return

    saving = true
    cfg.upstreams = upstreamsText
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
    cfg.wsOrigins = wsOriginsText
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)

    const roles: Record<string, { permissions: string[] }> = {}
    for (const r of roleList)
      if (r.name.trim()) roles[r.name.trim()] = { permissions: r.permissions }

    cfg.roles = Object.keys(roles).length ? roles : null
    const payload: WebServer = { ...cfg }
    delete payload.uiCurrentKdf

    if (loginDirty) {
      payload.uiPassword = await storedPassword()
      if (needCurrent) {
        payload.uiCurrentKdf = await deriveKDF(
          origUsername || 'admin',
          currentPass,
        )
      }
    } else {
      payload.uiPassword = ''
    }

    const ok = await saveSection('webserver', payload)
    if (ok) {
      currentPass = ''
      newPass = ''

      const { data } = await loadSection<WebServer>('webserver')
      if (data) {
        cfg = data
        orig = deepCopy(data)
        cfg.uiRoleHeader ??= ''
        orig.uiRoleHeader ??= ''
        applyAuth(data.uiPassword, true)
        await applyLiveAuth()
      } else {
        orig = deepCopy(cfg)
        applyAuth(cfg.uiPassword, true)
        await applyLiveAuth()
      }

      origUpstreams = upstreamsText
      origWSOrigins = wsOriginsText
      origRoles = deepCopy(cfg.roles)
      origRoleList = deepCopy(roleList)
      await restore()
    }

    saving = false
  }

  function onSave(e: Event) {
    e.preventDefault()
    void save()
  }
</script>

{#if loading}
  <Spinner color="primary" />
{:else if error}
  <p class="text-danger">{error}</p>
{:else if cfg && orig}
  <form onsubmit={onSave}>
    {#if pane === 'server'}
      <Card class="mb-3">
        <CardBody>
          <Row class="g-2">
            <Col md="6">
              <Input
                id="config.webserver.listenAddr"
                bind:value={cfg.listenAddr}
                original={orig.listenAddr}
                disabled={!canWrite}
                envVar="WEBSERVER_LISTEN_ADDR"
              />
            </Col>
            <Col md="6">
              <Input
                id="config.webserver.urlbase"
                bind:value={cfg.urlbase}
                original={orig.urlbase}
                disabled={!canWrite}
                envVar="WEBSERVER_URLBASE"
              />
            </Col>
            <Col md="6">
              <Input
                id="config.webserver.metrics"
                type="select"
                bind:value={cfg.metrics}
                original={orig.metrics}
                disabled={!canWrite}
                envVar="WEBSERVER_METRICS"
              />
            </Col>
            <Col md="6">
              <Input
                id="config.webserver.pprof"
                type="select"
                bind:value={cfg.pprof}
                original={orig.pprof}
                disabled={!canWrite}
                envVar="WEBSERVER_PPROF"
              />
            </Col>
            <Col md="12">
              <Input
                id="config.webserver.sslCertFile"
                bind:value={cfg.sslCertFile}
                original={orig.sslCertFile}
                disabled={!canWrite}
                envVar="WEBSERVER_SSL_CERT_FILE"
                browse="file"
                disableMkdir
              />
            </Col>
            <Col md="12">
              <Input
                id="config.webserver.sslKeyFile"
                bind:value={cfg.sslKeyFile}
                original={orig.sslKeyFile}
                disabled={!canWrite}
                envVar="WEBSERVER_SSL_KEY_FILE"
                browse="file"
                disableMkdir
              />
            </Col>
            <Col md="12">
              <Input
                id="config.webserver.logFile"
                bind:value={cfg.logFile}
                original={orig.logFile}
                disabled={!canWrite}
                envVar="WEBSERVER_LOG_FILE"
                browse="dir"
                browseFile="http.unpackerr.log"
              />
            </Col>
            <Col md="6">
              <Input
                id="config.webserver.logFiles"
                type="select"
                bind:value={cfg.logFiles}
                original={orig.logFiles}
                disabled={!canWrite}
                envVar="WEBSERVER_LOG_FILES"
                options={logFileCountChoices}
              />
            </Col>
            <Col md="6">
              <Input
                id="config.webserver.logFileMb"
                type="select"
                bind:value={cfg.logFileMb}
                original={orig.logFileMb}
                disabled={!canWrite}
                envVar="WEBSERVER_LOG_FILE_MB"
                options={logFileMbChoices}
              />
            </Col>
          </Row>
        </CardBody>
      </Card>
    {:else}
      <PageIntro id="pages.trust">
        {#if canWrite && profile.info?.clientIP}
          <Button
            size="sm"
            color="secondary"
            outline
            type="button"
            onclick={addThisIP}
          >
            {$_('buttons.AddThisIP')} ({profile.info.clientIP})
          </Button>
        {/if}
      </PageIntro>
      <Card class="mb-3">
        <CardHeader>{$_('pages.trust.Authorization')}</CardHeader>
        <CardBody>
          {#if filepathPass}
            <Alert color="secondary" class="py-2"
              >{$_('phrases.UiPasswordFilepath')}</Alert
            >
          {/if}
          <Row class="g-2">
            <Col md="6">
              <Input
                id="config.webserver.authType"
                type="select"
                bind:value={authType}
                original={origAuthType}
                disabled={!canWrite || authEnv}
                envVar="WEBSERVER_UI_PASSWORD"
                options={[
                  {
                    value: 'password',
                    name: $_('config.webserver.authType.options.password'),
                  },
                  {
                    value: 'header',
                    name: $_('config.webserver.authType.options.header'),
                    disabled: !upstreamAllowed,
                  },
                  {
                    value: 'noauth',
                    name: $_('config.webserver.authType.options.noauth'),
                    disabled: !upstreamAllowed,
                  },
                ]}
              />
            </Col>
            {#if needCurrent}
              <Col md="6">
                <Input
                  id="config.webserver.currentPassword"
                  type="password"
                  bind:value={currentPass}
                  original=""
                  disabled={!canWrite || authEnv}
                  envVar="WEBSERVER_UI_PASSWORD"
                  autocomplete="current-password"
                />
              </Col>
            {/if}
          </Row>
          <Row class="g-2 mt-1">
            <Col md="6">
              <Input
                id="config.webserver.upstreams"
                type="textarea"
                rows={3}
                bind:value={upstreamsText}
                original={origUpstreams}
                disabled={!canWrite}
                envVar="WEBSERVER_UPSTREAMS"
              />
            </Col>
            <Col md="6">
              <Input
                id="config.webserver.wsOrigins"
                type="textarea"
                rows={3}
                bind:value={wsOriginsText}
                original={origWSOrigins}
                disabled={!canWrite}
                envVar="WEBSERVER_WS_ORIGINS"
              />
            </Col>
          </Row>
          <Row class="g-2 mt-1">
            <Col md="6">
              {#if authType === 'header' || authType === 'noauth'}
                <Input
                  id="config.webserver.header"
                  type="select"
                  bind:value={headerName}
                  original={origHeader}
                  disabled={!canWrite || authEnv}
                  envVar="WEBSERVER_UI_PASSWORD"
                  options={headerOptions}
                />
              {:else}
                <Input
                  id="config.webserver.username"
                  bind:value={username}
                  original={origUsername}
                  disabled={!canWrite || authEnv}
                  envVar="WEBSERVER_UI_PASSWORD"
                />
                {#if reservedUser}
                  <p class="text-danger small mt-1">
                    {$_('phrases.ReservedUsername')}
                  </p>
                {/if}
              {/if}
            </Col>
            {#if authType === 'header'}
              <Col md="6">
                <Input
                  id="config.webserver.roleHeader"
                  helpKey="config.webserver.uiRoleHeader"
                  type="select"
                  bind:value={cfg.uiRoleHeader}
                  original={orig.uiRoleHeader ?? ''}
                  disabled={!canWrite}
                  envVar="WEBSERVER_UI_ROLE_HEADER"
                  options={roleHeaderOptions}
                />
              </Col>
            {:else if authType === 'password'}
              <Col md="6">
                <Input
                  id="config.webserver.newPassword"
                  type="password"
                  bind:value={newPass}
                  original=""
                  disabled={!canWrite || authEnv}
                  envVar="WEBSERVER_UI_PASSWORD"
                  autocomplete="new-password"
                />
              </Col>
            {/if}
          </Row>
        </CardBody>
      </Card>

      <Card class="mb-3">
        <CardHeader>
          <Row class="align-items-center">
            <Col>{$_('pages.settings.ApiKeys')}</Col>
            {#if canWrite}
              <Col xs="auto">
                <Button
                  type="button"
                  size="sm"
                  color="secondary"
                  outline
                  on:click={addKey}>{$_('buttons.AddKey')}</Button
                >
              </Col>
            {/if}
          </Row>
        </CardHeader>
        <CardBody>
          {#if cfg.apiKeys.length === 0}
            <p class="text-muted mb-0">{$_('phrases.NoApiKeys')}</p>
          {/if}
          {#each cfg.apiKeys as key, i (i)}
            <Card class="mb-2">
              <CardBody>
                <Input
                  id={`webserver-key-name-${i}`}
                  label={$_('config.webserver.keyName.label')}
                  bind:value={key.name}
                  original={orig.apiKeys[i]?.name}
                  disabled={!canWrite}
                  envVar={`WEBSERVER_API_KEYS_${i}_NAME`}
                />
                <Input
                  id={`webserver-key-${i}`}
                  type="password"
                  helpKey="config.webserver.key"
                  label={$_('config.webserver.key.label')}
                  bind:value={key.key}
                  original={orig.apiKeys[i]?.key}
                  disabled={!canWrite}
                  envVar={`WEBSERVER_API_KEYS_${i}_KEY`}
                >
                  {#snippet post()}
                    {#if canWrite}
                      <Button
                        type="button"
                        outline
                        class="px-3"
                        on:click={() => (key.key = newApiKey())}
                        >{$_('buttons.Generate')}</Button
                      >
                    {/if}
                  {/snippet}
                </Input>
                <FormGroup>
                  <Label>{$_('config.webserver.keyRoles.label')}</Label>
                  {#each roleNames as role, ri (ri)}
                    <FormCheck
                      inline
                      id={`key-${i}-role-${ri}`}
                      label={role}
                      checked={key.roles.includes(role)}
                      disabled={!canWrite ||
                        envHas(`WEBSERVER_API_KEYS_${i}_ROLES`)}
                      on:change={(e) =>
                        toggleKeyRole(
                          key,
                          role,
                          (e.currentTarget as HTMLInputElement).checked,
                        )}
                    />
                  {/each}
                </FormGroup>
                {#if canWrite}
                  <Row class="justify-content-end">
                    <Col xs="auto">
                      <Button
                        type="button"
                        size="sm"
                        color="danger"
                        outline
                        on:click={() => removeKey(i)}
                        >{$_('buttons.RemoveKey')}</Button
                      >
                    </Col>
                  </Row>
                {/if}
              </CardBody>
            </Card>
          {/each}
        </CardBody>
      </Card>

      <Card class="mb-3">
        <CardHeader>
          <Row class="align-items-center">
            <Col>{$_('pages.settings.CustomRoles')}</Col>
            {#if canWrite && !rolesEnv}
              <Col xs="auto">
                <Button
                  type="button"
                  size="sm"
                  color="secondary"
                  outline
                  on:click={addRole}>{$_('buttons.AddRole')}</Button
                >
              </Col>
            {/if}
          </Row>
        </CardHeader>
        <CardBody>
          <p class="text-muted small">
            {$_('phrases.AdminBuiltin')}
            <Badge color="primary">{RoleAdmin}</Badge>
          </p>
          {#each roleList as role, i (i)}
            <Card class="mb-2">
              <CardBody>
                <Input
                  id={`webserver-role-name-${i}`}
                  helpKey="config.webserver.roleName"
                  label={$_('config.webserver.roleName.label')}
                  bind:value={role.name}
                  original={origRoles ? (Object.keys(origRoles)[i] ?? '') : ''}
                  disabled={!canWrite || rolesEnv}
                  envVar="WEBSERVER_ROLES_*"
                  validate={(_id, v) =>
                    roleNameError(v) ||
                    roleNameDuplicateError(String(v), otherRoleNames(i))}
                />
                <FormGroup>
                  <Button
                    type="button"
                    color="link"
                    size="sm"
                    class="px-0 text-decoration-none"
                    on:click={() => togglePermOpen(i)}
                    aria-expanded={!!permOpen[i]}
                  >
                    {$_('config.webserver.permissions.label')}
                    <span class="ms-1" aria-hidden="true"
                      >{permOpen[i] ? '▾' : '▸'}</span
                    >
                  </Button>
                  {#if !permOpen[i]}
                    <div class="mt-1">
                      {#if role.permissions.length}
                        {#each role.permissions as perm (perm)}
                          <Badge
                            color="secondary"
                            class="me-1 mb-1 font-monospace">{perm}</Badge
                          >
                        {/each}
                      {:else}
                        <span class="text-muted small"
                          >{$_('phrases.NoPermissionsSelected')}</span
                        >
                      {/if}
                    </div>
                  {/if}
                  <Collapse isOpen={!!permOpen[i]}>
                    <Row class="mt-2">
                      {#each ALL_PERMISSIONS as perm (perm)}
                        <Col md="6">
                          <FormCheck
                            id={`role-${i}-${perm}`}
                            class="font-monospace small"
                            label={perm}
                            checked={role.permissions.includes(perm)}
                            disabled={!canWrite || rolesEnv}
                            on:change={(e) =>
                              toggleRolePerm(
                                role,
                                perm,
                                (e.currentTarget as HTMLInputElement).checked,
                              )}
                          />
                        </Col>
                      {/each}
                    </Row>
                  </Collapse>
                </FormGroup>
                {#if canWrite && !rolesEnv}
                  <Row class="justify-content-end">
                    <Col xs="auto">
                      <Button
                        type="button"
                        size="sm"
                        color="danger"
                        outline
                        on:click={() => removeRole(i)}
                        >{$_('buttons.RemoveRole')}</Button
                      >
                    </Col>
                  </Row>
                {/if}
              </CardBody>
            </Card>
          {/each}
        </CardBody>
      </Card>
    {/if}

    {#if canWrite}
      <SaveBar>
        <Button
          color="primary"
          type="submit"
          disabled={saving || authInvalid || rolesInvalid}
        >
          {#if saving}<Spinner size="sm" />{/if}
          <span class="ms-1">{$_('buttons.Save')}</span>
        </Button>
      </SaveBar>
    {/if}
  </form>
{/if}
