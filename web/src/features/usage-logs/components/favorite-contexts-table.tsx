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
import { Eye, Search, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
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

import {
  useDeleteFavoriteContext,
  useFavoriteContexts,
} from '../hooks/use-conversation-contexts'
import type { FavoriteConversationContext } from '../types'
import {
  ContextModelCell,
  ContextTimeCell,
  FavoriteContextRequestIdLink,
} from './conversation-context-column-cells'

const route = getRouteApi('/_authenticated/usage-logs/$section')

function buildFavoriteColumns(
  t: (key: string) => string,
  onDelete: (favorite: FavoriteConversationContext) => void
): ColumnDef<FavoriteConversationContext>[] {
  return [
    {
      id: 'request_id',
      header: t('Request ID'),
      cell: ({ row }) => (
        <FavoriteContextRequestIdLink
          favoriteId={row.original.id}
          requestId={row.original.request_id}
        />
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
      id: 'favorited_at',
      header: t('Favorited At'),
      cell: ({ row }) => (
        <ContextTimeCell timestamp={row.original.favorited_at} />
      ),
      size: 150,
    },
    {
      id: 'source_user_id',
      header: t('Source User'),
      cell: ({ row }) => {
        const userId = row.original.source_user_id
        if (userId == null) {
          return <span className='text-muted-foreground/40'>—</span>
        }
        return (
          <span className='font-mono text-xs tabular-nums'>
            {String(userId)}
          </span>
        )
      },
      size: 90,
    },
    {
      id: 'created_at',
      header: t('Created At'),
      cell: ({ row }) => (
        <ContextTimeCell timestamp={row.original.created_at} />
      ),
      size: 150,
    },
    {
      id: 'actions',
      header: t('Actions'),
      cell: ({ row }) => {
        const favorite = row.original
        const requestId = favorite.request_id
        return (
          <div className='flex items-center gap-1.5'>
            {requestId && (
              <Button
                variant='outline'
                size='sm'
                className='h-7 gap-1.5'
                render={
                  <Link
                    to='/usage-logs/favorite-context/$id'
                    params={{ id: String(favorite.id) }}
                  />
                }
              >
                <Eye className='size-3.5' />
                {t('View')}
              </Button>
            )}
            <Button
              variant='ghost'
              size='sm'
              className='text-destructive hover:text-destructive h-7 gap-1.5'
              onClick={() => onDelete(favorite)}
            >
              <Trash2 className='size-3.5' />
              {t('Delete')}
            </Button>
          </div>
        )
      },
      enableHiding: false,
      size: 150,
    },
  ]
}

export function FavoriteContextsTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [deleteTarget, setDeleteTarget] =
    useState<FavoriteConversationContext | null>(null)
  const deleteMutation = useDeleteFavoriteContext()

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

  const { data, isLoading, isFetching } = useFavoriteContexts({
    page: pagination.pageIndex + 1,
    pageSize: pagination.pageSize,
    requestId: requestId || undefined,
  })

  const items = useMemo(
    () => data?.items ?? ([] as FavoriteConversationContext[]),
    [data]
  )

  const handleDelete = useMemo(
    () => (favorite: FavoriteConversationContext) => {
      setDeleteTarget(favorite)
    },
    []
  )

  const columns = useMemo(
    () => buildFavoriteColumns(t, handleDelete),
    [t, handleDelete]
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

  const handleConfirmDelete = () => {
    if (!deleteTarget) return
    deleteMutation.mutate(deleteTarget.id, {
      onSuccess: (result) => {
        setDeleteTarget(null)
        if (result.success) {
          toast.success(t('Favorite context deleted'))
        } else {
          toast.error(result.message || t('Failed to delete favorite context'))
        }
      },
      onError: () => {
        setDeleteTarget(null)
        toast.error(t('Failed to delete favorite context'))
      },
    })
  }

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
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No favorite contexts found')}
        emptyDescription={t(
          'Favorite a conversation context from its detail page to keep it here permanently.'
        )}
        skeletonKeyPrefix='favorite-context-skeleton'
        applyHeaderSize
        toolbar={toolbar}
        tableClassName='[&_[data-slot=table]]:text-[13px] [&_[data-slot=table]_td]:text-[13px] [&_[data-slot=table]_td_*]:text-[13px] [&_[data-slot=table]_th]:text-[13px] [&_[data-slot=table]_th_*]:text-[13px]'
        getColumnClassName={() => 'py-2'}
      />

      <ConfirmDialog
        open={deleteTarget != null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t('Delete favorite context')}
        desc={t(
          'This favorite snapshot will be permanently removed. This action cannot be undone.'
        )}
        confirmText={t('Delete')}
        destructive
        handleConfirm={handleConfirmDelete}
        isLoading={deleteMutation.isPending}
      />
    </>
  )
}
