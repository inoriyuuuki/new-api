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
import { Link2, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatTimestampToDate } from '@/lib/format'

import type { CredentialProfile } from '../types'

type CredentialProfilesTableProps = {
  profiles: CredentialProfile[]
  loading: boolean
  onCreate: () => void
  onEdit: (profile: CredentialProfile) => void
  onManage: (profile: CredentialProfile) => void
  onApply: (profile: CredentialProfile) => void
  onDelete: (profile: CredentialProfile) => void
}

function ActionButton({
  label,
  onClick,
  destructive,
  children,
}: {
  label: string
  onClick: () => void
  destructive?: boolean
  children: React.ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={onClick}
            aria-label={label}
            className={destructive ? 'text-destructive' : undefined}
          >
            {children}
          </Button>
        }
      />
      <TooltipContent>
        <p>{label}</p>
      </TooltipContent>
    </Tooltip>
  )
}

export function CredentialProfilesTable({
  profiles,
  loading,
  onCreate,
  onEdit,
  onManage,
  onApply,
  onDelete,
}: CredentialProfilesTableProps) {
  const { t } = useTranslation()

  const columns: StaticDataTableColumn<CredentialProfile>[] = [
    {
      id: 'name',
      header: t('Name'),
      cell: (row) => (
        <div className='flex max-w-72 min-w-0 flex-col gap-0.5'>
          <span className='truncate text-sm font-medium'>{row.name}</span>
          {row.remark ? (
            <span className='text-muted-foreground truncate text-xs'>
              {row.remark}
            </span>
          ) : null}
        </div>
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
      id: 'bound',
      header: t('Bound'),
      cell: (row) => (
        <span className='text-sm tabular-nums'>{row.bound_count}</span>
      ),
    },
    {
      id: 'out-of-sync',
      header: t('Out of sync'),
      cell: (row) =>
        row.out_of_sync_count > 0 ? (
          <span className='text-destructive text-sm font-medium tabular-nums'>
            {row.out_of_sync_count}
          </span>
        ) : (
          <span className='text-muted-foreground text-sm tabular-nums'>
            {row.out_of_sync_count}
          </span>
        ),
    },
    {
      id: 'updated',
      header: t('Updated'),
      cell: (row) => (
        <span className='text-muted-foreground text-sm whitespace-nowrap tabular-nums'>
          {formatTimestampToDate(row.updated_time)}
        </span>
      ),
    },
    {
      id: 'actions',
      header: t('Actions'),
      className: 'w-40',
      cell: (row) => (
        <div className='flex items-center gap-0.5'>
          <ActionButton label={t('Edit Profile')} onClick={() => onEdit(row)}>
            <Pencil className='size-4' />
          </ActionButton>
          <ActionButton
            label={t('Manage Bound Channels')}
            onClick={() => onManage(row)}
          >
            <Link2 className='size-4' />
          </ActionButton>
          <ActionButton label={t('Refresh')} onClick={() => onApply(row)}>
            <RefreshCw className='size-4' />
          </ActionButton>
          <ActionButton
            label={t('Delete')}
            onClick={() => onDelete(row)}
            destructive
          >
            <Trash2 className='size-4' />
          </ActionButton>
        </div>
      ),
    },
  ]

  return (
    <div className='min-h-0 flex-1'>
      <StaticDataTable
        columns={columns}
        data={loading ? [] : profiles}
        getRowKey={(row) => row.id}
        emptyContent={
          <div className='flex flex-col items-center gap-3 py-10'>
            <p className='text-sm font-medium'>
              {t('No credential profiles yet')}
            </p>
            <p className='text-muted-foreground max-w-md text-sm'>
              {t(
                'Create a credential profile to batch refresh API keys and Base URLs across channels.'
              )}
            </p>
            <Button onClick={onCreate}>
              <Plus className='size-4' />
              {t('New Profile')}
            </Button>
          </div>
        }
      />
    </div>
  )
}
