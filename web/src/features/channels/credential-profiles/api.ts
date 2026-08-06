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
import { api, type ApiRequestConfig } from '@/lib/api'

import type {
  CredentialProfileApplyResponse,
  CredentialProfileChannelsResponse,
  CredentialProfileCreatePayload,
  CredentialProfileListResponse,
  CredentialProfileSetChannelsResponse,
  CredentialProfileSimpleResponse,
  CredentialProfileUpdatePayload,
  SetCredentialProfileChannelsPayload,
} from './types'

const credentialProfileRequestConfig = (
  config: ApiRequestConfig = {}
): ApiRequestConfig => ({
  ...config,
  skipBusinessError: true,
  skipErrorHandler: true,
})

/**
 * List saved credential profiles. The response never contains API keys.
 */
export async function getCredentialProfiles(): Promise<CredentialProfileListResponse> {
  const res = await api.get(
    '/api/channel/credential-profiles',
    credentialProfileRequestConfig()
  )
  return res.data
}

/**
 * Create a credential profile. `key` is required and is only sent here and on
 * update; it is never returned by list/detail endpoints.
 */
export async function createCredentialProfile(
  payload: CredentialProfileCreatePayload
): Promise<CredentialProfileSimpleResponse> {
  const res = await api.post(
    '/api/channel/credential-profiles',
    payload,
    credentialProfileRequestConfig()
  )
  return res.data
}

/**
 * Update a credential profile. An omitted/empty `key` keeps the stored key;
 * an explicit empty `base_url` clears it.
 */
export async function updateCredentialProfile(
  id: number,
  payload: CredentialProfileUpdatePayload
): Promise<CredentialProfileSimpleResponse> {
  const res = await api.put(
    `/api/channel/credential-profiles/${id}`,
    payload,
    credentialProfileRequestConfig()
  )
  return res.data
}

/**
 * Delete a credential profile. The backend rejects deletion while the profile
 * still has bound channels.
 */
export async function deleteCredentialProfile(
  id: number
): Promise<CredentialProfileSimpleResponse> {
  const res = await api.delete(
    `/api/channel/credential-profiles/${id}`,
    credentialProfileRequestConfig()
  )
  return res.data
}

/**
 * List the channels bound to a profile together with their `in_sync` state.
 */
export async function getCredentialProfileChannels(
  id: number
): Promise<CredentialProfileChannelsResponse> {
  const res = await api.get(
    `/api/channel/credential-profiles/${id}/channels`,
    credentialProfileRequestConfig()
  )
  return res.data
}

/**
 * Replace the full set of channels bound to a profile. Newly bound channels
 * are synced immediately by the backend.
 */
export async function setCredentialProfileChannels(
  id: number,
  channelIds: number[]
): Promise<CredentialProfileSetChannelsResponse> {
  const payload: SetCredentialProfileChannelsPayload = {
    channel_ids: channelIds,
  }
  const res = await api.put(
    `/api/channel/credential-profiles/${id}/channels`,
    payload,
    credentialProfileRequestConfig()
  )
  return res.data
}

/**
 * Refresh the credentials of every channel bound to the profile. Returns
 * per-channel results; the response never echoes API keys.
 */
export async function applyCredentialProfile(
  id: number,
  dryRun: boolean
): Promise<CredentialProfileApplyResponse> {
  const res = await api.post(
    `/api/channel/credential-profiles/${id}/apply`,
    { dry_run: dryRun },
    credentialProfileRequestConfig()
  )
  return res.data
}
