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
// ============================================================================
// Credential Profile Types
// ============================================================================

/**
 * A saved credential profile (name + API key + base URL) that can be bound to
 * multiple channels and applied to refresh them in bulk.
 *
 * API keys are never returned by the API; the list/detail payloads only carry
 * metadata and per-channel sync state.
 */
export interface CredentialProfile {
  id: number
  name: string
  base_url?: string | null
  remark?: string | null
  bound_count: number
  out_of_sync_count: number
  created_time: number
  updated_time: number
}

export interface CredentialProfileCreatePayload {
  name: string
  key: string
  base_url?: string
  remark?: string
}

export interface CredentialProfileUpdatePayload {
  name?: string
  /** Omit or leave empty to keep the stored API key unchanged. */
  key?: string
  /** An explicit empty string clears the Base URL. */
  base_url?: string
  remark?: string
}

export interface SetCredentialProfileChannelsPayload {
  /** Full replacement set of bound channel ids. */
  channel_ids: number[]
}

/**
 * A channel bound to a credential profile. The backend augments the channel
 * metadata with `in_sync` (whether the channel key/base_url already match the
 * profile).
 */
export interface CredentialProfileChannel {
  id: number
  name: string
  type: number
  status: number
  base_url?: string | null
  models?: string | null
  group?: string | null
  tag?: string | null
  is_multi_key: boolean
  multi_key_size: number
  in_sync: boolean
}

export interface CredentialProfileApplyResultItem {
  channel_id: number
  name: string
  type: number
  base_url?: string
  is_multi_key: boolean
  success: boolean
  /** Whether the channel credentials were (or would be, for a dry run) written. */
  synced?: boolean
  error?: string
}

export interface CredentialProfileApplyResult {
  /** true for a dry-run preview, false for a real apply. */
  dry_run?: boolean
  total: number
  succeeded: number
  failed: number
  /** Number of channels whose credentials were (or would be) actually updated. */
  synced?: number
  results: CredentialProfileApplyResultItem[]
}

export interface CredentialProfileBindResultItem {
  channel_id: number
  name?: string
  type?: number
  base_url?: string
  is_multi_key?: boolean
  success: boolean
  synced?: boolean
  error?: string
}

export interface CredentialProfileBindResult {
  total: number
  added: number
  removed: number
  succeeded: number
  failed: number
  /** Channel ids that could not be bound because they already belong to another profile. */
  conflict_ids?: number[]
  results: CredentialProfileBindResultItem[]
}

// ============================================================================
// API Response Envelopes
// ============================================================================

export interface CredentialProfileListResponse {
  success: boolean
  message?: string
  data?: CredentialProfile[]
}

export interface CredentialProfileChannelsResponse {
  success: boolean
  message?: string
  data?: CredentialProfileChannel[]
}

export interface CredentialProfileSetChannelsResponse {
  success: boolean
  message?: string
  data?: CredentialProfileBindResult
}

export interface CredentialProfileApplyResponse {
  success: boolean
  message?: string
  data?: CredentialProfileApplyResult
}

export interface CredentialProfileSimpleResponse {
  success: boolean
  message?: string
  data?: unknown
}
