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
import { Loader2, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { getCredentialProfiles, deleteCredentialProfile } from '../api'
import { credentialProfilesQueryKeys, getErrorMessage } from '../lib'
import type { CredentialProfile } from '../types'
import { CredentialProfileApplyDialog } from './credential-profile-apply-dialog'
import { CredentialProfileChannelsDialog } from './credential-profile-channels-dialog'
import { CredentialProfileDialog } from './credential-profile-dialog'
import { CredentialProfilesTable } from './credential-profiles-table'

export function CredentialProfilesPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((s) => s.auth.user)
  const canSensitiveWrite = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )

  const [createOpen, setCreateOpen] = useState(false)
  const [editingProfile, setEditingProfile] =
    useState<CredentialProfile | null>(null)
  const [managingProfile, setManagingProfile] =
    useState<CredentialProfile | null>(null)
  const [applyingProfile, setApplyingProfile] =
    useState<CredentialProfile | null>(null)
  const [deletingProfile, setDeletingProfile] =
    useState<CredentialProfile | null>(null)
  const [deleting, setDeleting] = useState(false)

  const profilesQuery = useQuery({
    queryKey: credentialProfilesQueryKeys.list(),
    queryFn: getCredentialProfiles,
  })

  if (!canSensitiveWrite) {
    return (
      <p className='text-muted-foreground py-16 text-center text-sm'>
        {t('No permission to perform this action')}
      </p>
    )
  }

  const profiles = profilesQuery.data?.data ?? []

  const handleDelete = async () => {
    if (!deletingProfile) return
    setDeleting(true)
    try {
      const res = await deleteCredentialProfile(deletingProfile.id)
      if (!res.success) {
        toast.error(res.message || t('Failed to delete profile'))
        return
      }
      toast.success(t('Profile deleted successfully'))
      queryClient.invalidateQueries({
        queryKey: credentialProfilesQueryKeys.all,
      })
      setDeletingProfile(null)
    } catch (e) {
      toast.error(getErrorMessage(e, t('Failed to delete profile')))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground max-w-xl text-sm'>
          {t(
            'Credential profiles bundle an API key and Base URL. Bind channels to a profile, then refresh them all with one click.'
          )}
        </p>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className='size-4' />
          {t('New Profile')}
        </Button>
      </div>

      {profilesQuery.isError ? (
        <p className='text-destructive py-16 text-center text-sm'>
          {t('Failed to load credential profiles')}
        </p>
      ) : (
        <CredentialProfilesTable
          profiles={profiles}
          loading={profilesQuery.isLoading}
          onCreate={() => setCreateOpen(true)}
          onEdit={setEditingProfile}
          onManage={setManagingProfile}
          onApply={setApplyingProfile}
          onDelete={setDeletingProfile}
        />
      )}

      <CredentialProfileDialog
        mode='create'
        profile={null}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />

      <CredentialProfileDialog
        mode='edit'
        profile={editingProfile}
        open={editingProfile !== null}
        onOpenChange={(open) => {
          if (!open) setEditingProfile(null)
        }}
      />

      <CredentialProfileChannelsDialog
        profile={managingProfile}
        open={managingProfile !== null}
        onOpenChange={(open) => {
          if (!open) setManagingProfile(null)
        }}
      />

      <CredentialProfileApplyDialog
        profile={applyingProfile}
        open={applyingProfile !== null}
        onOpenChange={(open) => {
          if (!open) setApplyingProfile(null)
        }}
      />

      <ConfirmDialog
        open={deletingProfile !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingProfile(null)
        }}
        title={t('Delete Profile')}
        desc={t(
          'This will permanently delete the credential profile "{{name}}". Channels bound to it are not affected.',
          { name: deletingProfile?.name ?? '' }
        )}
        destructive
        isLoading={deleting}
        confirmText={
          <span className='inline-flex items-center gap-2'>
            {deleting ? <Loader2 className='animate-spin' /> : null}
            {t('Delete')}
          </span>
        }
        handleConfirm={handleDelete}
      />
    </div>
  )
}
