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
import { useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import { channelsQueryKeys, getChannelTypeLabel } from '../../lib'
import { applyCredentialProfile } from '../api'
import { credentialProfilesQueryKeys, getErrorMessage } from '../lib'
import type {
  CredentialProfile,
  CredentialProfileApplyResult,
  CredentialProfileApplyResultItem,
} from '../types'

type ApplyPhase = 'preview' | 'applied'

type CredentialProfilePreview = {
  profileId: number
  result: CredentialProfileApplyResult
}

type CredentialProfileApplyDialogProps = {
  profile: CredentialProfile | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function ChannelCell({ name, id }: { name: string; id: number }) {
  return (
    <div className='flex min-w-0 flex-col gap-0.5'>
      <span className='truncate text-sm font-medium'>{name}</span>
      <span className='text-muted-foreground text-xs'>#{id}</span>
    </div>
  )
}

function MultiKeyCell({ isMultiKey }: { isMultiKey: boolean }) {
  const { t } = useTranslation()
  if (!isMultiKey) {
    return <span className='text-muted-foreground text-sm'>{t('None')}</span>
  }
  return <Badge variant='outline'>{t('Multi-key')}</Badge>
}

function ResultBadge({
  item,
  isPreview,
}: {
  item: CredentialProfileApplyResultItem
  isPreview: boolean
}) {
  const { t } = useTranslation()

  if (!item.success) {
    return (
      <span className='text-destructive text-sm'>
        {item.error || t('Failed')}
      </span>
    )
  }

  if (item.synced) {
    return (
      <StatusBadge variant='success' copyable={false}>
        {isPreview ? t('To update') : t('Updated')}
      </StatusBadge>
    )
  }

  return (
    <StatusBadge variant='success' copyable={false}>
      {t('In sync')}
    </StatusBadge>
  )
}

export function CredentialProfileApplyDialog({
  profile,
  open,
  onOpenChange,
}: CredentialProfileApplyDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [phase, setPhase] = useState<ApplyPhase>('preview')
  const [preview, setPreview] = useState<CredentialProfilePreview | null>(null)
  const [result, setResult] = useState<CredentialProfileApplyResult | null>(
    null
  )
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [applying, setApplying] = useState(false)
  const previewRequestIdRef = useRef(0)
  const profileId = profile?.id ?? null
  const currentPreview =
    preview?.profileId === profileId ? preview.result : null

  const runPreview = useCallback(async () => {
    if (!open || profileId === null) return

    const requestId = ++previewRequestIdRef.current
    setPreviewLoading(true)
    setPreviewError(null)
    try {
      const res = await applyCredentialProfile(profileId, true)
      if (requestId !== previewRequestIdRef.current) return

      if (res.success && res.data) {
        setPreview({ profileId, result: res.data })
      } else {
        setPreviewError(res.message || t('Failed to refresh credentials'))
      }
    } catch (e) {
      if (requestId !== previewRequestIdRef.current) return
      setPreviewError(getErrorMessage(e, t('Failed to refresh credentials')))
    } finally {
      if (requestId === previewRequestIdRef.current) {
        setPreviewLoading(false)
      }
    }
  }, [open, profileId, t])

  useEffect(() => {
    if (!open || profileId === null) {
      previewRequestIdRef.current += 1
      return
    }

    setPhase('preview')
    setResult(null)
    setPreview(null)
    setPreviewError(null)
    void runPreview()

    return () => {
      previewRequestIdRef.current += 1
    }
  }, [open, profileId, runPreview])

  const handleApply = async () => {
    if (!profile || !currentPreview) return
    setApplying(true)
    try {
      const res = await applyCredentialProfile(profile.id, false)
      if (res.success && res.data) {
        setResult(res.data)
        setPhase('applied')
        toast.success(t('Credential refresh completed'))
        queryClient.invalidateQueries({
          queryKey: credentialProfilesQueryKeys.all,
        })
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
      } else {
        toast.error(res.message || t('Failed to refresh credentials'))
      }
    } catch (e) {
      toast.error(getErrorMessage(e, t('Failed to refresh credentials')))
    } finally {
      setApplying(false)
    }
  }

  const isPreview = phase === 'preview'
  const data = isPreview ? currentPreview : result
  const total = data?.total ?? 0
  const succeeded = data?.succeeded ?? 0
  const failed = data?.failed ?? 0
  const synced = data?.synced ?? 0

  const columns: StaticDataTableColumn<CredentialProfileApplyResultItem>[] = [
    {
      id: 'channel',
      header: t('Channel'),
      cell: (row) => <ChannelCell name={row.name} id={row.channel_id} />,
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
        <span className='text-muted-foreground max-w-56 truncate text-sm'>
          {row.base_url || '—'}
        </span>
      ),
    },
    {
      id: 'multi-key',
      header: t('Multi-key'),
      cell: (row) => <MultiKeyCell isMultiKey={row.is_multi_key} />,
    },
    {
      id: 'result',
      header: t('Status'),
      cell: (row) => <ResultBadge item={row} isPreview={isPreview} />,
    },
  ]

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => !v && onOpenChange(false)}
      title={
        isPreview
          ? t('Credential Refresh Preview')
          : t('Credential Refresh Results')
      }
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        isPreview ? (
          <>
            <Button variant='outline' onClick={() => onOpenChange(false)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={handleApply}
              disabled={applying || previewLoading || total === 0}
            >
              {applying ? (
                <RefreshCw className='animate-spin' />
              ) : (
                <RefreshCw />
              )}
              {t('Apply to {{count}} channels', { count: total })}
            </Button>
          </>
        ) : (
          <Button onClick={() => onOpenChange(false)}>{t('Close')}</Button>
        )
      }
    >
      <div className='flex flex-wrap items-center gap-2'>
        <StatusBadge variant='neutral' copyable={false}>
          {t('Total')}: {total}
        </StatusBadge>
        <StatusBadge variant='success' copyable={false}>
          {t('Succeeded')}: {succeeded}
        </StatusBadge>
        <StatusBadge variant='danger' copyable={false}>
          {t('Failed')}: {failed}
        </StatusBadge>
        <StatusBadge variant='info' copyable={false}>
          {isPreview ? t('To update') : t('Updated')}: {synced}
        </StatusBadge>
      </div>

      {renderBody()}
    </Dialog>
  )

  function renderBody() {
    if (previewLoading && currentPreview === null) {
      return (
        <div className='text-muted-foreground flex items-center justify-center gap-2 py-8 text-sm'>
          <Loader2 className='animate-spin' />
          {t('Loading...')}
        </div>
      )
    }
    if (previewError !== null) {
      return (
        <div className='flex flex-col items-center gap-3 py-8 text-center'>
          <p className='text-destructive text-sm'>{previewError}</p>
          <Button variant='outline' onClick={() => void runPreview()}>
            {t('Retry')}
          </Button>
        </div>
      )
    }
    if (data === null) {
      return null
    }
    if (total === 0) {
      return (
        <p className='text-muted-foreground py-8 text-center text-sm'>
          {t(
            'This profile has no bound channels. Use Manage Bound Channels to add channels.'
          )}
        </p>
      )
    }
    return (
      <div className='max-h-80 overflow-y-auto rounded-md border'>
        <StaticDataTable
          columns={columns}
          data={data.results ?? []}
          getRowKey={(row) => row.channel_id}
        />
      </div>
    )
  }
}
