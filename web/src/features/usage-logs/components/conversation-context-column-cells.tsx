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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

/**
 * Monospace link to the standalone conversation context detail page.
 * Only rendered when a request id is present.
 */
export function ContextRequestIdLink({
  requestId,
  className,
}: {
  requestId: string | undefined
  className?: string
}) {
  if (!requestId) {
    return <span className='text-muted-foreground/40'>—</span>
  }
  return (
    <Link
      to='/usage-logs/context/$requestId'
      params={{ requestId }}
      className={cn(
        'text-primary hover:underline font-mono text-xs break-all',
        className
      )}
    >
      {requestId}
    </Link>
  )
}

/**
 * Monospace link to the standalone favorite context detail page. Favorite
 * snapshots live in the main database and remain readable even after the
 * original context (DB A) has been cleaned up.
 */
export function FavoriteContextRequestIdLink({
  favoriteId,
  requestId,
  className,
}: {
  favoriteId: number
  requestId: string | undefined
  className?: string
}) {
  if (!requestId) {
    return <span className='text-muted-foreground/40'>—</span>
  }
  return (
    <Link
      to='/usage-logs/favorite-context/$id'
      params={{ id: String(favoriteId) }}
      className={cn(
        'text-primary hover:underline font-mono text-xs break-all',
        className
      )}
    >
      {requestId}
    </Link>
  )
}

/**
 * Formatted timestamp cell used by both context lists.
 */
export function ContextTimeCell({ timestamp }: { timestamp: number }) {
  if (!timestamp) {
    return <span className='text-muted-foreground/40'>—</span>
  }
  return (
    <span className='font-mono text-xs whitespace-nowrap tabular-nums'>
      {formatTimestampToDate(timestamp)}
    </span>
  )
}

/**
 * Truncated model name cell.
 */
export function ContextModelCell({ model }: { model: string }) {
  if (!model) {
    return <span className='text-muted-foreground/40'>—</span>
  }
  return (
    <span className='block max-w-[12rem] truncate text-xs' title={model}>
      {model}
    </span>
  )
}

/**
 * Localized Yes/No badge for boolean context fields.
 */
export function ContextBooleanBadge({ value }: { value: boolean }) {
  const { t } = useTranslation()
  return (
    <Badge variant={value ? 'default' : 'secondary'}>
      {value ? t('Yes') : t('No')}
    </Badge>
  )
}

/**
 * Favorite state badge.
 */
export function ContextFavoriteBadge({ isFavorite }: { isFavorite: boolean }) {
  const { t } = useTranslation()
  return (
    <Badge variant={isFavorite ? 'default' : 'secondary'}>
      {isFavorite ? t('Favorited') : t('Not favorited')}
    </Badge>
  )
}
