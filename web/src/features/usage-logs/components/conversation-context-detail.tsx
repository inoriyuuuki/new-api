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
import { useNavigate } from '@tanstack/react-router'
import { Check, Copy, Loader2, Star, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  useConversationContextDetail,
  useDeleteFavoriteContext,
  useFavoriteContextDetail,
  useFavoriteConversationContext,
} from '../hooks/use-conversation-contexts'
import type { ConversationContext, FavoriteConversationContext } from '../types'
import { ConversationMessagesView } from './conversation-messages-view'

/**
 * Best-effort pretty-printing: if the value parses as JSON it is re-serialized
 * with indentation, otherwise the raw text is kept (e.g. plain text bodies).
 */
function formatJsonText(value: string | undefined): string {
  if (!value) return ''
  const trimmed = value.trim()
  if (!trimmed) return ''
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  } catch {
    return value
  }
}

function DetailRow({
  label,
  children,
  mono = false,
}: {
  label: string
  children: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='grid min-w-0 grid-cols-[6rem_minmax(0,1fr)] gap-2 text-sm sm:grid-cols-[8rem_minmax(0,1fr)] sm:gap-3'>
      <span className='text-muted-foreground min-w-0 text-xs'>{label}</span>
      <span className={cn('min-w-0 text-xs break-all', mono && 'font-mono')}>
        {children}
      </span>
    </div>
  )
}

function JsonBlock({
  title,
  value,
}: {
  title: string
  value: string | undefined
}) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const formatted = formatJsonText(value)

  return (
    <Card>
      <CardHeader className='flex-row items-center justify-between gap-2 border-b'>
        <CardTitle className='text-sm font-semibold'>{title}</CardTitle>
        <Button
          variant='ghost'
          size='sm'
          className='h-7 gap-1.5'
          onClick={() => {
            if (formatted) {
              void copyToClipboard(formatted)
            }
          }}
          disabled={!formatted}
        >
          {copiedText === formatted ? (
            <Check className='size-3.5 text-green-600' />
          ) : (
            <Copy className='size-3.5' />
          )}
          {t('Copy')}
        </Button>
      </CardHeader>
      <CardContent className='px-0 pb-0'>
        <pre className='bg-muted/40 max-h-[480px] overflow-auto px-4 py-3 font-mono text-[11px] leading-relaxed break-all whitespace-pre-wrap'>
          {formatted || t('Empty')}
        </pre>
      </CardContent>
    </Card>
  )
}

function DetailSkeleton() {
  return (
    <div className='space-y-4'>
      <Card>
        <CardHeader>
          <Skeleton className='h-5 w-40' />
        </CardHeader>
        <CardContent className='space-y-3'>
          {[1, 2, 3, 4, 5, 6].map((i) => (
            <Skeleton key={i} className='h-4 w-full' />
          ))}
        </CardContent>
      </Card>
      <Skeleton className='h-64 w-full' />
    </div>
  )
}

function MetaGrid({
  context,
}: {
  context: ConversationContext | FavoriteConversationContext
}) {
  const { t } = useTranslation()
  const isStreamLabel = context.is_stream ? t('Yes') : t('No')
  const favoriteLabel = context.is_favorite
    ? t('Favorited')
    : t('Not favorited')
  const isFavoriteSnapshot = 'favorited_at' in context

  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-sm font-semibold'>
          {t('Request Metadata')}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-2.5'>
        <DetailRow label={t('Request ID')} mono>
          {context.request_id || '-'}
        </DetailRow>
        <DetailRow label={t('Log ID')}>{context.log_id || '-'}</DetailRow>
        <DetailRow label={t('User ID')}>{context.user_id || '-'}</DetailRow>
        <DetailRow label={t('Created At')}>
          {context.created_at ? formatTimestampToDate(context.created_at) : '-'}
        </DetailRow>
        {isFavoriteSnapshot && (
          <DetailRow label={t('Favorited At')}>
            {context.favorited_at
              ? formatTimestampToDate(context.favorited_at)
              : '-'}
          </DetailRow>
        )}
        {isFavoriteSnapshot && (
          <DetailRow label={t('Source User')}>
            {context.source_user_id != null
              ? String(context.source_user_id)
              : '-'}
          </DetailRow>
        )}
        <DetailRow label={t('Request Path')} mono>
          {context.request_path || '-'}
        </DetailRow>
        <DetailRow label={t('Relay Format')}>
          {context.relay_format || '-'}
        </DetailRow>
        <DetailRow label={t('Model')}>{context.model_name || '-'}</DetailRow>
        <DetailRow label={t('Response Status')} mono>
          {context.response_status != null
            ? String(context.response_status)
            : '-'}
        </DetailRow>
        <DetailRow label={t('Stream')}>{isStreamLabel}</DetailRow>
        <DetailRow label={t('Capture Status')}>
          {context.capture_status || '-'}
        </DetailRow>
        <div className='flex items-center gap-2 pt-1'>
          <Label className='text-muted-foreground text-xs'>
            {t('Favorite')}
          </Label>
          <Badge variant={context.is_favorite ? 'default' : 'secondary'}>
            {favoriteLabel}
          </Badge>
        </div>
      </CardContent>
    </Card>
  )
}

type ConversationContextDetailProps =
  | { requestId: string; favoriteId?: never }
  | { requestId?: never; favoriteId: number }

/**
 * Standalone conversation context detail page.
 *
 * Regular mode (`requestId`): reads the live record from the context database
 * (DB A) and lets the owner favorite it.
 *
 * Favorite mode (`favoriteId`): reads the favorite snapshot from the main
 * database (DB B). The snapshot stays readable even after the original context
 * in DB A has been cleaned up; it can only be removed manually.
 */
export function ConversationContextDetail({
  requestId,
  favoriteId,
}: ConversationContextDetailProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isFavoriteMode = favoriteId != null

  // Both queries are always mounted; the inactive one is disabled so it
  // never fetches. This keeps hook order stable across both modes.
  const regularQuery = useConversationContextDetail(requestId ?? '')
  const favoriteQuery = useFavoriteContextDetail(favoriteId ?? 0)
  const favoriteMutation = useFavoriteConversationContext()
  const deleteMutation = useDeleteFavoriteContext()
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  const data = isFavoriteMode ? favoriteQuery.data : regularQuery.data
  const isLoading = isFavoriteMode
    ? favoriteQuery.isLoading
    : regularQuery.isLoading
  const isError = isFavoriteMode ? favoriteQuery.isError : regularQuery.isError
  const error = isFavoriteMode ? favoriteQuery.error : regularQuery.error

  if (isLoading) {
    return <DetailSkeleton />
  }

  if (isError || !data) {
    return (
      <div className='text-muted-foreground rounded-md border p-6 text-center text-sm'>
        {t('Failed to load conversation context')}
        {error instanceof Error && error.message ? `: ${error.message}` : ''}
      </div>
    )
  }

  if (isFavoriteMode) {
    const isDeleting = deleteMutation.isPending
    const handleDelete = () => {
      deleteMutation.mutate(favoriteId, {
        onSuccess: (result) => {
          setDeleteConfirmOpen(false)
          if (result.success) {
            toast.success(t('Favorite context deleted'))
            void navigate({
              to: '/usage-logs/$section',
              params: { section: 'favorite-contexts' },
            })
          } else {
            toast.error(
              result.message || t('Failed to delete favorite context')
            )
          }
        },
        onError: () => {
          setDeleteConfirmOpen(false)
          toast.error(t('Failed to delete favorite context'))
        },
      })
    }

    return (
      <div className='space-y-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex items-center gap-2'>
            <Badge variant='default'>{t('Favorited')}</Badge>
          </div>
          <Button
            variant='destructive'
            size='sm'
            onClick={() => setDeleteConfirmOpen(true)}
            disabled={isDeleting}
          >
            {isDeleting ? (
              <Loader2 className='size-3.5 animate-spin' />
            ) : (
              <Trash2 className='size-3.5' />
            )}
            {t('Delete favorite context')}
          </Button>
        </div>

        <MetaGrid context={data} />

        <JsonBlock title={t('Request Parameters')} value={data.request_meta} />

        <ConversationMessagesView
          requestBody={data.request_body}
          responseBody={data.response_body}
        />

        <JsonBlock title={t('Request Body')} value={data.request_body} />
        <JsonBlock title={t('Response Body')} value={data.response_body} />

        <ConfirmDialog
          open={deleteConfirmOpen}
          onOpenChange={setDeleteConfirmOpen}
          title={t('Delete favorite context')}
          desc={t(
            'This favorite snapshot will be permanently removed. This action cannot be undone.'
          )}
          confirmText={t('Delete')}
          destructive
          handleConfirm={handleDelete}
          isLoading={isDeleting}
        />
      </div>
    )
  }

  const isFavoriting = favoriteMutation.isPending

  const handleFavorite = () => {
    if (data.is_favorite || isFavoriting) return
    favoriteMutation.mutate(data.request_id)
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex items-center gap-2'>
          <Badge variant={data.is_favorite ? 'default' : 'secondary'}>
            {data.is_favorite ? t('Favorited') : t('Not favorited')}
          </Badge>
        </div>
        <Button
          variant={data.is_favorite ? 'secondary' : 'default'}
          size='sm'
          onClick={handleFavorite}
          disabled={data.is_favorite || isFavoriting}
        >
          {isFavoriting ? (
            <Loader2 className='size-3.5 animate-spin' />
          ) : (
            <Star className='size-3.5' />
          )}
          {data.is_favorite ? t('Favorited') : t('Add to favorites')}
        </Button>
      </div>

      <MetaGrid context={data} />

      <JsonBlock title={t('Request Parameters')} value={data.request_meta} />

      <ConversationMessagesView
        requestBody={data.request_body}
        responseBody={data.response_body}
      />

      <JsonBlock title={t('Request Body')} value={data.request_body} />
      <JsonBlock title={t('Response Body')} value={data.response_body} />
    </div>
  )
}
