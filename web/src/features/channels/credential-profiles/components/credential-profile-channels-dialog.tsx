/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Search } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useDebounce } from '@/hooks'

import { getChannels, searchChannels } from '../../api'
import { CHANNEL_STATUS_CONFIG } from '../../constants'
import { channelsQueryKeys, getChannelTypeLabel } from '../../lib'
import {
  getCredentialProfileChannels,
  setCredentialProfileChannels,
} from '../api'
import { credentialProfilesQueryKeys, getErrorMessage } from '../lib'
import type {
  CredentialProfile,
  CredentialProfileBindResult,
  CredentialProfileBindResultItem,
  CredentialProfileChannel,
} from '../types'
import type { Channel } from '../../types'

// The picker shows every channel in one scrollable list; the page size only
// bounds each API request (backend caps at 100) and the page count is a
// defensive cap against runaway loops.
const CHANNEL_PICKER_PAGE_SIZE = 100
const CHANNEL_PICKER_MAX_PAGES = 100

type DialogView = 'picker' | 'results'

type CredentialProfileChannelsDialogProps = {
  profile: CredentialProfile | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function BindResultCell({ item }: { item: CredentialProfileBindResultItem }) {
  const { t } = useTranslation()

  if (!item.success) {
    return (
      <span className='text-destructive text-sm'>
        {item.error || t('Failed')}
      </span>
    )
  }

  return (
    <StatusBadge variant='success' copyable={false}>
      {t('Success')}
    </StatusBadge>
  )
}

export function CredentialProfileChannelsDialog({
  profile,
  open,
  onOpenChange,
}: CredentialProfileChannelsDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [view, setView] = useState<DialogView>('picker')
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword, 300)
  const [selectedIds, setSelectedIds] = useState<Set<number> | null>(null)
  const [saving, setSaving] = useState(false)
  const [bindResult, setBindResult] =
    useState<CredentialProfileBindResult | null>(null)

  const channelsQuery = useQuery({
    queryKey: credentialProfilesQueryKeys.channels(profile?.id ?? -1),
    queryFn: () => getCredentialProfileChannels(profile?.id ?? -1),
    enabled: open && profile !== null,
  })

  const pickerQuery = useQuery({
    queryKey: [
      'channel-credential-profiles',
      'picker',
      profile?.id,
      debouncedKeyword,
    ],
    queryFn: async () => {
      const trimmed = debouncedKeyword.trim()
      const byId = new Map<number, Channel>()
      let page = 1
      while (page <= CHANNEL_PICKER_MAX_PAGES) {
        const res = trimmed
          ? await searchChannels({
              keyword: trimmed,
              p: page,
              page_size: CHANNEL_PICKER_PAGE_SIZE,
            })
          : await getChannels({
              p: page,
              page_size: CHANNEL_PICKER_PAGE_SIZE,
            })
        const items = res.data?.items ?? []
        for (const item of items) {
          byId.set(item.id, item)
        }
        const total = res.data?.total ?? items.length
        if (items.length < CHANNEL_PICKER_PAGE_SIZE || byId.size >= total) {
          break
        }
        page += 1
      }
      const items = [...byId.values()]
      return { data: { items, total: items.length } }
    },
    enabled: open && profile !== null,
  })

  const boundChannels = channelsQuery.data?.data
  const pickerItems = pickerQuery.data?.data?.items ?? []

  useEffect(() => {
    if (open) {
      setView('picker')
      setKeyword('')
      setSelectedIds(null)
      setBindResult(null)
    }
  }, [open, profile?.id])

  useEffect(() => {
    if (selectedIds === null && boundChannels !== undefined) {
      setSelectedIds(new Set(boundChannels.map((channel) => channel.id)))
    }
  }, [selectedIds, boundChannels])

  const tableScrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    tableScrollRef.current?.scrollTo({ top: 0 })
  }, [debouncedKeyword])

  const boundById = useMemo(() => {
    const map = new Map<number, CredentialProfileChannel>()
    for (const channel of boundChannels ?? []) {
      map.set(channel.id, channel)
    }
    return map
  }, [boundChannels])

  const selectedCount = selectedIds?.size ?? 0
  const outOfSyncCount = useMemo(
    () => (boundChannels ?? []).filter((channel) => !channel.in_sync).length,
    [boundChannels]
  )

  const toggleChannel = (id: number) => {
    setSelectedIds((prev) => {
      if (prev === null) return prev
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const toggleAllVisible = (ids: number[]) => {
    setSelectedIds((prev) => {
      if (prev === null) return prev
      const next = new Set(prev)
      const allSelected = ids.every((id) => next.has(id))
      for (const id of ids) {
        if (allSelected) {
          next.delete(id)
        } else {
          next.add(id)
        }
      }
      return next
    })
  }

  const handleSave = async () => {
    if (!profile || selectedIds === null) return
    setSaving(true)
    try {
      const res = await setCredentialProfileChannels(profile.id, [
        ...selectedIds,
      ])
      if (!res.success) {
        toast.error(res.message || t('Failed to update bound channels'))
        return
      }
      // The binding itself is persisted even when per-channel sync fails, so
      // refresh derived data in both outcomes.
      queryClient.invalidateQueries({
        queryKey: credentialProfilesQueryKeys.all,
      })
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })

      if (res.data && res.data.failed > 0) {
        setBindResult(res.data)
        setView('results')
        toast.error(
          t('Some channels failed to bind. Review the details below.')
        )
        return
      }

      toast.success(t('Bound channels updated successfully'))
      onOpenChange(false)
    } catch (e) {
      toast.error(getErrorMessage(e, t('Failed to update bound channels')))
    } finally {
      setSaving(false)
    }
  }

  const otherProfileBinding = (channelId: number): boolean => {
    if (profile === null) return false
    const channel = pickerItems.find((item) => item.id === channelId)
    if (!channel) return false
    return (
      channel.credential_profile_id !== null &&
      channel.credential_profile_id !== undefined &&
      channel.credential_profile_id !== profile.id
    )
  }

  // Channels that can be toggled: rows not already bound to another profile.
  const pickerSelectableIds = pickerItems
    .filter((channel) => !otherProfileBinding(channel.id))
    .map((channel) => channel.id)

  const columns: StaticDataTableColumn<(typeof pickerItems)[number]>[] = [
    {
      id: 'select',
      header: (
        <Checkbox
          checked={
            pickerSelectableIds.length > 0 &&
            pickerSelectableIds.every((id) => selectedIds?.has(id))
          }
          indeterminate={
            pickerSelectableIds.some((id) => selectedIds?.has(id)) &&
            !pickerSelectableIds.every((id) => selectedIds?.has(id))
          }
          disabled={selectedIds === null || pickerSelectableIds.length === 0}
          onCheckedChange={() => toggleAllVisible(pickerSelectableIds)}
          aria-label={t('Select All Visible')}
        />
      ),
      cell: (row) => {
        const disabled = otherProfileBinding(row.id)
        return (
          <Checkbox
            checked={disabled ? false : (selectedIds?.has(row.id) ?? false)}
            disabled={selectedIds === null || disabled}
            onCheckedChange={() => toggleChannel(row.id)}
            aria-label={t('Select channel {{name}}', { name: row.name })}
          />
        )
      },
    },
    {
      id: 'channel',
      header: t('Channel'),
      cell: (row) => (
        <div className='flex min-w-0 flex-col gap-0.5'>
          <span className='truncate text-sm font-medium'>{row.name}</span>
          <span className='text-muted-foreground text-xs'>#{row.id}</span>
        </div>
      ),
    },
    {
      id: 'type',
      header: t('Type'),
      cell: (row) => (
        <span className='text-muted-foreground text-sm'>
          {t(getChannelTypeLabel(row.type))}
        </span>
      ),
    },
    {
      id: 'base-url',
      header: t('Base URL'),
      cell: (row) => (
        <span className='text-muted-foreground max-w-52 truncate text-sm'>
          {row.base_url || '—'}
        </span>
      ),
    },
    {
      id: 'status',
      header: t('Status'),
      cell: (row) => {
        const config =
          CHANNEL_STATUS_CONFIG[
            row.status as keyof typeof CHANNEL_STATUS_CONFIG
          ] ?? CHANNEL_STATUS_CONFIG[0]
        return (
          <StatusBadge variant={config.variant} copyable={false}>
            {t(config.label)}
          </StatusBadge>
        )
      },
    },
    {
      id: 'sync',
      header: t('Sync'),
      cell: (row) => {
        if (otherProfileBinding(row.id)) {
          return (
            <Tooltip>
              <TooltipTrigger
                render={
                  <StatusBadge variant='warning' copyable={false}>
                    {t('Bound to another profile')}
                  </StatusBadge>
                }
              />
              <TooltipContent>
                <p>
                  {t('Channel already bound to another credential profile')}
                </p>
              </TooltipContent>
            </Tooltip>
          )
        }
        const bound = boundById.get(row.id)
        if (!bound) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return bound.in_sync ? (
          <StatusBadge variant='success' copyable={false}>
            {t('In sync')}
          </StatusBadge>
        ) : (
          <StatusBadge variant='warning' copyable={false}>
            {t('Out of sync')}
          </StatusBadge>
        )
      },
    },
  ]

  const resultColumns: StaticDataTableColumn<CredentialProfileBindResultItem>[] =
    [
      {
        id: 'channel',
        header: t('Channel'),
        cell: (row) => <ChannelCellView item={row} />,
      },
      {
        id: 'status',
        header: t('Status'),
        cell: (row) => <BindResultCell item={row} />,
      },
    ]

  const bindFailed = bindResult?.failed ?? 0
  const conflictIds = bindResult?.conflict_ids ?? []

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => !v && onOpenChange(false)}
      title={t('Manage Bound Channels')}
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        view === 'results' ? (
          <>
            <Button variant='outline' onClick={() => setView('picker')}>
              {t('Back')}
            </Button>
            <Button onClick={() => onOpenChange(false)}>{t('Close')}</Button>
          </>
        ) : (
          <>
            <Button variant='outline' onClick={() => onOpenChange(false)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={handleSave}
              disabled={saving || selectedIds === null}
            >
              {saving ? <Loader2 className='animate-spin' /> : null}
              {t('Save')}
            </Button>
          </>
        )
      }
    >
      {view === 'results' && bindResult ? (
        <>
          <p className='text-destructive text-sm'>
            {t('Some channels failed to bind. Review the details below.')}
          </p>

          {conflictIds.length > 0 ? (
            <p className='text-warning text-sm'>
              {t(
                'Channels already bound to another credential profile: {{ids}}',
                {
                  ids: conflictIds.join(', '),
                }
              )}
            </p>
          ) : null}

          <div className='flex flex-wrap items-center gap-2'>
            <StatusBadge variant='neutral' copyable={false}>
              {t('Total')}: {bindResult.total}
            </StatusBadge>
            <StatusBadge variant='success' copyable={false}>
              {t('Succeeded')}: {bindResult.succeeded}
            </StatusBadge>
            <StatusBadge variant='danger' copyable={false}>
              {t('Failed')}: {bindFailed}
            </StatusBadge>
            <StatusBadge variant='neutral' copyable={false}>
              {t('Removed')}: {bindResult.removed}
            </StatusBadge>
          </div>

          <div className='max-h-96 overflow-y-auto rounded-md border'>
            <StaticDataTable
              columns={resultColumns}
              data={bindResult.results ?? []}
              getRowKey={(row) => row.channel_id}
              emptyContent={
                <p className='text-muted-foreground text-sm'>
                  {t('No matching channels')}
                </p>
              }
            />
          </div>
        </>
      ) : (
        <>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Saving replaces the entire bound channel set. Channels you uncheck will no longer be refreshed by this profile.'
            )}
          </p>

          <div className='flex flex-wrap items-center gap-2'>
            <StatusBadge variant='neutral' copyable={false}>
              {t('Bound')}: {boundChannels?.length ?? 0}
            </StatusBadge>
            <StatusBadge variant='warning' copyable={false}>
              {t('Out of sync')}: {outOfSyncCount}
            </StatusBadge>
            <StatusBadge variant='info' copyable={false}>
              {t('{{count}} selected', { count: selectedCount })}
            </StatusBadge>
          </div>

          <div className='relative'>
            <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
            <Input
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              placeholder={t('Search...')}
              className='pl-8'
            />
          </div>

          {pickerQuery.isPending && pickerItems.length === 0 ? (
            <div className='border-border/60 flex items-center justify-center gap-2 rounded-md border py-10 text-sm text-muted-foreground'>
              <Loader2 className='animate-spin size-4' />
              {t('Loading')}
            </div>
          ) : (
            <div
              ref={tableScrollRef}
              className='max-h-96 overflow-y-auto rounded-md border'
            >
              <StaticDataTable
                columns={columns}
                data={pickerItems}
                getRowKey={(row) => row.id}
                emptyContent={
                  <p className='text-muted-foreground text-sm'>
                    {t('No matching channels')}
                  </p>
                }
              />
            </div>
          )}

        </>
      )}
    </Dialog>
  )
}

function ChannelCellView({ item }: { item: CredentialProfileBindResultItem }) {
  const { t } = useTranslation()
  return (
    <div className='flex min-w-0 flex-col gap-0.5'>
      <span className='truncate text-sm font-medium'>
        {item.name || t('Channel')}
      </span>
      <span className='text-muted-foreground text-xs'>#{item.channel_id}</span>
    </div>
  )
}
