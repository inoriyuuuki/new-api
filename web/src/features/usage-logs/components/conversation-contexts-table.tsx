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
import { getRouteApi, Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { Eye, Search } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  DataTableViewOptions,
  useDataTable,
  useDebouncedColumnFilter,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { useConversationContexts } from '../hooks/use-conversation-contexts'
import type { ConversationContext } from '../types'
import {
  ContextBooleanBadge,
  ContextFavoriteBadge,
  ContextModelCell,
  ContextRequestIdLink,
  ContextTimeCell,
} from './conversation-context-column-cells'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

function buildContextColumns(
  t: (key: string) => string,
  isAdminView: boolean
): ColumnDef<ConversationContext>[] {
  const columns: ColumnDef<ConversationContext>[] = [
    {
      id: 'request_id',
      header: t('Request ID'),
      cell: ({ row }) => (
        <ContextRequestIdLink requestId={row.original.request_id} />
      ),
      enableHiding: false,
      size: 200,
    },
    {
      id: 'model_name',
      header: t('Model'),
      cell: ({ row }) => <ContextModelCell model={row.original.model_name} />,
      size: 140,
    },
    {
      id: 'request_path',
      header: t('Request Path'),
      cell: ({ row }) => (
        <span
          className='block max-w-[14rem] truncate font-mono text-xs'
          title={row.original.request_path}
        >
          {row.original.request_path || '—'}
        </span>
      ),
      size: 160,
    },
    {
      id: 'response_status',
      header: t('Response Status'),
      cell: ({ row }) => {
        const status = row.original.response_status
        if (status == null) {
          return <span className='text-muted-foreground/40'>—</span>
        }
        return (
          <span
            className={cn(
              'font-mono text-xs tabular-nums',
              status >= 400 ? 'text-red-600 dark:text-red-400' : ''
            )}
          >
            {String(status)}
          </span>
        )
      },
      size: 100,
    },
    {
      id: 'is_stream',
      header: t('Stream'),
      cell: ({ row }) => <ContextBooleanBadge value={row.original.is_stream} />,
      size: 80,
    },
    {
      id: 'is_favorite',
      header: t('Favorite'),
      cell: ({ row }) => (
        <ContextFavoriteBadge isFavorite={row.original.is_favorite} />
      ),
      size: 100,
    },
    {
      id: 'created_at',
      header: t('Created At'),
      cell: ({ row }) => (
        <ContextTimeCell timestamp={row.original.created_at} />
      ),
      size: 150,
    },
  ]

  if (isAdminView) {
    columns.splice(1, 0, {
      id: 'user_id',
      header: t('User ID'),
      cell: ({ row }) => {
        const userId = row.original.user_id
        if (userId == null) {
          return <span className='text-muted-foreground/40'>—</span>
        }
        return (
          <span className='font-mono text-xs tabular-nums'>
            {String(userId)}
          </span>
        )
      },
      size: 80,
    })
  }

  columns.push({
    id: 'actions',
    header: t('Actions'),
    cell: ({ row }) => {
      const requestId = row.original.request_id
      if (!requestId) return null
      return (
        <Button
          variant='outline'
          size='sm'
          className='h-7 gap-1.5'
          render={
            <Link to='/usage-logs/context/$requestId' params={{ requestId }} />
          }
        >
          <Eye className='size-3.5' />
          {t('View Context')}
        </Button>
      )
    },
    enableHiding: false,
    size: 130,
  })

  return columns
}

export function ConversationContextsTable() {
  const { t } = useTranslation()
  const { isAdminView } = useLogsViewScope()
  const currentUserId = useAuthStore((state) => state.auth.user?.id)
  const isMobile = useMediaQuery('(max-width: 640px)')

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 20 : 50 },
    globalFilter: { enabled: false },
    columnFilters: [
      {
        columnId: 'request_id',
        searchKey: 'requestId',
        type: 'string' as const,
      },
    ],
  })

  const requestId =
    (columnFilters.find((filter) => filter.id === 'request_id')?.value as
      | string
      | undefined) ?? ''

  const { data, isLoading, isFetching } = useConversationContexts({
    page: pagination.pageIndex + 1,
    pageSize: pagination.pageSize,
    requestId: requestId || undefined,
    // Admin "All" view lists every user's contexts; any other view (admin
    // "Only Mine" or a regular user) is scoped to the current user.
    userId: isAdminView ? undefined : currentUserId,
  })

  const items = useMemo(
    () => data?.items ?? ([] as ConversationContext[]),
    [data]
  )
  const columns = useMemo(
    () => buildContextColumns(t, isAdminView),
    [t, isAdminView]
  )

  const { table } = useDataTable({
    data: items,
    columns,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    manualPagination: true,
    manualFiltering: true,
    enableRowSelection: false,
    totalCount: data?.total ?? 0,
    ensurePageInRange,
  })

  const requestIdFilter = useDebouncedColumnFilter({
    columnFilters,
    columnId: 'request_id',
    onColumnFiltersChange,
  })

  const toolbar = (
    <div className='bg-card/50 flex flex-wrap items-center gap-2 rounded-lg border p-2.5 sm:p-3'>
      <div className='relative min-w-0 flex-1 sm:max-w-xs'>
        <Search className='text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2' />
        <Input
          placeholder={t('Search by request ID')}
          value={requestIdFilter.inputValue}
          onChange={requestIdFilter.onChange}
          onCompositionStart={requestIdFilter.onCompositionStart}
          onCompositionEnd={requestIdFilter.onCompositionEnd}
          className='h-8 pr-3 pl-8 text-sm'
        />
      </div>
      <div className='ms-auto flex items-center gap-1.5'>
        <DataTableViewOptions table={table} />
      </div>
    </div>
  )

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No conversation contexts found')}
      emptyDescription={t(
        'Conversation context is captured alongside usage logs and appears here automatically.'
      )}
      skeletonKeyPrefix='conversation-context-skeleton'
      applyHeaderSize
      toolbar={toolbar}
      tableClassName='[&_[data-slot=table]]:text-[13px] [&_[data-slot=table]_td]:text-[13px] [&_[data-slot=table]_td_*]:text-[13px] [&_[data-slot=table]_th]:text-[13px] [&_[data-slot=table]_th_*]:text-[13px]'
      getColumnClassName={() => 'py-2'}
    />
  )
}
