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
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import { createCredentialProfile, updateCredentialProfile } from '../api'
import {
  buildCredentialProfileCreatePayload,
  buildCredentialProfileUpdatePayload,
  createEmptyCredentialProfileForm,
  credentialProfileToFormState,
  credentialProfilesQueryKeys,
  getErrorMessage,
  validateCredentialProfileForm,
  type CredentialProfileFormState,
} from '../lib'
import type { CredentialProfile } from '../types'

type CredentialProfileDialogProps = {
  mode: 'create' | 'edit'
  profile: CredentialProfile | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CredentialProfileDialog({
  mode,
  profile,
  open,
  onOpenChange,
}: CredentialProfileDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<CredentialProfileFormState>(
    createEmptyCredentialProfileForm
  )
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (open) {
      setForm(
        mode === 'edit' && profile
          ? credentialProfileToFormState(profile)
          : createEmptyCredentialProfileForm()
      )
    }
  }, [open, mode, profile])

  const setField = <K extends keyof CredentialProfileFormState>(
    key: K,
    value: CredentialProfileFormState[K]
  ) => {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const handleSave = async () => {
    const errors = validateCredentialProfileForm(form, mode)
    if (errors.length > 0) {
      toast.error(t(errors[0]))
      return
    }

    setSaving(true)
    try {
      if (mode === 'create') {
        const res = await createCredentialProfile(
          buildCredentialProfileCreatePayload(form)
        )
        if (!res.success) {
          toast.error(res.message || t('Failed to create profile'))
          return
        }
        toast.success(t('Profile created successfully'))
      } else {
        if (!profile) return
        const res = await updateCredentialProfile(
          profile.id,
          buildCredentialProfileUpdatePayload(form)
        )
        if (!res.success) {
          toast.error(res.message || t('Failed to update profile'))
          return
        }
        toast.success(t('Profile updated successfully'))
      }

      queryClient.invalidateQueries({
        queryKey: credentialProfilesQueryKeys.all,
      })
      onOpenChange(false)
    } catch (e) {
      toast.error(
        getErrorMessage(
          e,
          mode === 'create'
            ? t('Failed to create profile')
            : t('Failed to update profile')
        )
      )
    } finally {
      setSaving(false)
    }
  }

  const isEdit = mode === 'edit'

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => !v && onOpenChange(false)}
      title={isEdit ? t('Edit Profile') : t('New Profile')}
      contentClassName='sm:max-w-lg'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave} disabled={saving}>
            {saving ? <Loader2 className='animate-spin' /> : null}
            {t('Save')}
          </Button>
        </>
      }
    >
      <div className='flex flex-col gap-3'>
        <div className='flex flex-col gap-2'>
          <Label htmlFor='credential-profile-name'>{t('Profile name')}</Label>
          <Input
            id='credential-profile-name'
            value={form.name}
            onChange={(e) => setField('name', e.target.value)}
            placeholder={t('Profile name')}
            autoComplete='off'
          />
        </div>

        <div className='flex flex-col gap-2'>
          {isEdit ? (
            <label className='flex cursor-pointer items-center gap-2'>
              <Checkbox
                checked={form.updateKey}
                onCheckedChange={(checked) => setField('updateKey', checked)}
              />
              <span className='text-sm font-medium'>{t('Update API Key')}</span>
            </label>
          ) : (
            <Label htmlFor='credential-profile-key'>{t('API Key')}</Label>
          )}

          {(!isEdit || form.updateKey) && (
            <div className='flex flex-col gap-2'>
              <Textarea
                id='credential-profile-key'
                value={form.key}
                onChange={(e) => setField('key', e.target.value)}
                placeholder={t('API Key')}
                rows={4}
                autoComplete='off'
                spellCheck={false}
                className='font-mono'
              />
              {isEdit ? (
                <p className='text-muted-foreground text-xs'>
                  {t('Leave empty to keep the current API key')}
                </p>
              ) : null}
            </div>
          )}
        </div>

        <div className='flex flex-col gap-2'>
          {isEdit ? (
            <label className='flex cursor-pointer items-center gap-2'>
              <Checkbox
                checked={form.updateBaseUrl}
                onCheckedChange={(checked) =>
                  setField('updateBaseUrl', checked)
                }
              />
              <span className='text-sm font-medium'>
                {t('Update Base URL')}
              </span>
            </label>
          ) : (
            <Label htmlFor='credential-profile-base-url'>{t('Base URL')}</Label>
          )}

          {(!isEdit || form.updateBaseUrl) && (
            <div className='flex flex-col gap-2'>
              <Input
                id='credential-profile-base-url'
                value={form.baseUrl}
                onChange={(e) => setField('baseUrl', e.target.value)}
                placeholder={t('Base URL')}
                autoComplete='off'
              />
              {isEdit ? (
                <p className='text-muted-foreground text-xs'>
                  {t('Leave empty to clear the Base URL')}
                </p>
              ) : null}
            </div>
          )}
        </div>

        <div className='flex flex-col gap-2'>
          <Label htmlFor='credential-profile-remark'>{t('Remark')}</Label>
          <Input
            id='credential-profile-remark'
            value={form.remark}
            onChange={(e) => setField('remark', e.target.value)}
            placeholder={t('Remark')}
          />
        </div>
      </div>
    </Dialog>
  )
}
