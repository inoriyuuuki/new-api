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
import type {
  CredentialProfile,
  CredentialProfileCreatePayload,
  CredentialProfileUpdatePayload,
} from './types'

// ============================================================================
// Credential Profile Form
// ============================================================================

export interface CredentialProfileFormState {
  name: string
  /** On edit: whether to send a new API key (leave empty to keep current). */
  updateKey: boolean
  key: string
  /** On edit: whether to send base_url (empty string clears it). */
  updateBaseUrl: boolean
  baseUrl: string
  remark: string
}

// Validation error keys (i18n keys, rendered through t() at the call site).
const CREDENTIAL_PROFILE_ERRORS = {
  NAME_REQUIRED: 'Profile name is required',
  KEY_REQUIRED: 'API key is required',
} as const

export function createEmptyCredentialProfileForm(): CredentialProfileFormState {
  return {
    name: '',
    updateKey: true,
    key: '',
    updateBaseUrl: false,
    baseUrl: '',
    remark: '',
  }
}

export function credentialProfileToFormState(
  profile: CredentialProfile
): CredentialProfileFormState {
  return {
    name: profile.name,
    updateKey: false,
    key: '',
    updateBaseUrl: false,
    baseUrl: profile.base_url ?? '',
    remark: profile.remark ?? '',
  }
}

export function validateCredentialProfileForm(
  form: CredentialProfileFormState,
  mode: 'create' | 'edit'
): string[] {
  const errors: string[] = []

  if (!form.name.trim()) {
    errors.push(CREDENTIAL_PROFILE_ERRORS.NAME_REQUIRED)
  }

  const keyRequired = mode === 'create' || form.updateKey
  if (keyRequired && !form.key.trim()) {
    errors.push(CREDENTIAL_PROFILE_ERRORS.KEY_REQUIRED)
  }

  return errors
}

/**
 * Build the create payload. `key` is always sent; `base_url` is only sent when
 * non-empty (an empty value is the same as no Base URL for a new profile).
 */
export function buildCredentialProfileCreatePayload(
  form: CredentialProfileFormState
): CredentialProfileCreatePayload {
  const payload: CredentialProfileCreatePayload = {
    name: form.name.trim(),
    key: form.key,
  }

  const baseUrl = form.baseUrl.trim()
  if (baseUrl) {
    payload.base_url = baseUrl
  }

  const remark = form.remark.trim()
  if (remark) {
    payload.remark = remark
  }

  return payload
}

/**
 * Build the update payload. `key` is omitted unless the user opted to change
 * it; an explicit empty `base_url` clears the stored value.
 */
export function buildCredentialProfileUpdatePayload(
  form: CredentialProfileFormState
): CredentialProfileUpdatePayload {
  const payload: CredentialProfileUpdatePayload = {
    name: form.name.trim(),
  }

  if (form.updateKey) {
    payload.key = form.key
  }

  if (form.updateBaseUrl) {
    payload.base_url = form.baseUrl
  }

  payload.remark = form.remark.trim()

  return payload
}

// ============================================================================
// Query Keys
// ============================================================================

/**
 * Query keys for credential profile data. API keys are never part of any query
 * key or cached value.
 */
export const credentialProfilesQueryKeys = {
  all: ['channel-credential-profiles'] as const,
  lists: () => [...credentialProfilesQueryKeys.all, 'list'] as const,
  list: () => [...credentialProfilesQueryKeys.lists()] as const,
  channels: (id: number) =>
    [...credentialProfilesQueryKeys.all, 'channels', id] as const,
}

// ============================================================================
// Shared Helpers
// ============================================================================

export function getErrorMessage(e: unknown, fallback: string): string {
  const err = e as {
    response?: { data?: { message?: string } }
    message?: string
  }
  return err?.response?.data?.message || err?.message || fallback
}
