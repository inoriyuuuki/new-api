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
import { api } from '@/lib/api'

import { buildQueryParams } from './lib/query-params'
import type {
  GetLogsParams,
  GetLogsResponse,
  GetLogStatsParams,
  GetLogStatsResponse,
  GetMidjourneyLogsParams,
  GetTaskLogsParams,
  UserInfo,
  GetConversationContextsParams,
  GetFavoriteContextsParams,
  ConversationContextListResponse,
  ConversationContext,
  FavoriteConversationContext,
} from './types'

// ============================================================================
// Generic API Helpers
// ============================================================================

function buildApiPath(endpoint: string, isAdmin: boolean): string {
  return isAdmin ? endpoint : `${endpoint}/self`
}

async function fetchLogs<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogsResponse> {
  const paramRecord = params as unknown as Record<string, unknown>
  const queryParams = buildQueryParams({
    p: paramRecord.p || 1,
    page_size: paramRecord.page_size || 20,
    ...params,
  })
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}?${queryParams}`)
  return res.data
}

async function fetchLogStats<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogStatsResponse> {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}/stat?${queryParams}`)
  return res.data
}

// ============================================================================
// Common Log APIs
// ============================================================================

export const getAllLogs = (params: GetLogsParams = {}) =>
  fetchLogs('/api/log', params, true)

export const getUserLogs = (
  params: Omit<GetLogsParams, 'username' | 'channel'> = {}
) => fetchLogs('/api/log', params, false)

export const getLogStats = (params: GetLogStatsParams = {}) =>
  fetchLogStats('/api/log', params, true)

export const getUserLogStats = (
  params: Omit<GetLogStatsParams, 'username' | 'channel'> = {}
) => fetchLogStats('/api/log', params, false)

export async function getUserInfo(
  userId: number
): Promise<{ success: boolean; message?: string; data?: UserInfo }> {
  const res = await api.get(`/api/user/${userId}`)
  return res.data
}

// ============================================================================
// MjProxy (Drawing) Logs API
// ============================================================================

export const getAllMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, true)

export const getUserMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, false)

// ============================================================================
// Task Logs API
// ============================================================================

export const getAllTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, true)

export const getUserTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, false)

// ============================================================================
// Conversation Context APIs
// ============================================================================
//
// Backend contract (unified UserAuth API, no /self variant):
//   GET    /api/conversation-context?p&page_size&request_id&user_id
//   GET    /api/conversation-context/item/:request_id
//   POST   /api/conversation-context/item/:request_id/favorite
//   GET    /api/conversation-context/favorites?p&page_size
//   GET    /api/conversation-context/favorites/:id
//   DELETE /api/conversation-context/favorites/:id

const CONVERSATION_CONTEXT_PATH = '/api/conversation-context'

export const getConversationContexts = async (
  params: GetConversationContextsParams = {}
): Promise<ConversationContextListResponse> => {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const res = await api.get(
    `${CONVERSATION_CONTEXT_PATH}?${queryParams.toString()}`
  )
  return res.data
}

export const getConversationContextDetail = async (
  requestId: string
): Promise<{
  success: boolean
  message?: string
  data?: ConversationContext
}> => {
  const res = await api.get(
    `${CONVERSATION_CONTEXT_PATH}/item/${encodeURIComponent(requestId)}`
  )
  return res.data
}

export const favoriteConversationContext = async (
  requestId: string
): Promise<{ success: boolean; message?: string; data?: unknown }> => {
  const res = await api.post(
    `${CONVERSATION_CONTEXT_PATH}/item/${encodeURIComponent(requestId)}/favorite`
  )
  return res.data
}

export const getFavoriteContexts = async (
  params: GetFavoriteContextsParams = {}
): Promise<ConversationContextListResponse> => {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const res = await api.get(
    `${CONVERSATION_CONTEXT_PATH}/favorites?${queryParams.toString()}`
  )
  return res.data
}

export const getFavoriteContextDetail = async (
  id: number
): Promise<{
  success: boolean
  message?: string
  data?: FavoriteConversationContext
}> => {
  const res = await api.get(
    `${CONVERSATION_CONTEXT_PATH}/favorites/${encodeURIComponent(String(id))}`
  )
  return res.data
}

export const deleteFavoriteContext = async (
  id: number
): Promise<{ success: boolean; message?: string; data?: unknown }> => {
  const res = await api.delete(
    `${CONVERSATION_CONTEXT_PATH}/favorites/${encodeURIComponent(String(id))}`
  )
  return res.data
}
