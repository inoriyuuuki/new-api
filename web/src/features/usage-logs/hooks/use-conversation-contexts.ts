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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import {
  deleteFavoriteContext,
  favoriteConversationContext,
  getConversationContextDetail,
  getConversationContexts,
  getFavoriteContextDetail,
  getFavoriteContexts,
} from '../api'
import type { ConversationContext, FavoriteConversationContext } from '../types'

export const conversationContextKeys = {
  all: ['conversation-contexts'] as const,
  list: (params: unknown) => ['conversation-contexts', 'list', params] as const,
  detail: (requestId: string) =>
    ['conversation-contexts', 'detail', requestId] as const,
  favoriteDetail: (id: number) =>
    ['conversation-contexts', 'favorite-detail', id] as const,
  favorites: (params: unknown) =>
    ['conversation-contexts', 'favorites', params] as const,
}

type PaginatedResult<T> = {
  items: T[]
  total: number
  page: number
  page_size: number
}

const EMPTY_PAGINATED: PaginatedResult<never> = {
  items: [],
  total: 0,
  page: 1,
  page_size: 20,
}

/**
 * Paginated conversation context list (captured request/response context).
 * Admin can scope to a specific user via `userId`; regular users are always
 * scoped to themselves by the backend.
 */
export function useConversationContexts(params: {
  page: number
  pageSize: number
  requestId?: string
  userId?: number
}) {
  const { page, pageSize, requestId, userId } = params
  return useQuery({
    queryKey: conversationContextKeys.list({
      page,
      pageSize,
      requestId,
      userId,
    }),
    queryFn: async (): Promise<PaginatedResult<ConversationContext>> => {
      const result = await getConversationContexts({
        p: page,
        page_size: pageSize,
        request_id: requestId || undefined,
        user_id: userId,
      })
      if (!result.success) {
        throw new Error(
          result.message || 'Failed to load conversation contexts'
        )
      }
      return (
        (result.data as PaginatedResult<ConversationContext>) ?? EMPTY_PAGINATED
      )
    },
    placeholderData: (previousData) => previousData,
  })
}

/**
 * Paginated favorite context list. Favorites always belong to the currently
 * logged-in user.
 */
export function useFavoriteContexts(params: {
  page: number
  pageSize: number
  requestId?: string
}) {
  const { page, pageSize, requestId } = params
  return useQuery({
    queryKey: conversationContextKeys.favorites({ page, pageSize, requestId }),
    queryFn: async (): Promise<
      PaginatedResult<FavoriteConversationContext>
    > => {
      const result = await getFavoriteContexts({
        p: page,
        page_size: pageSize,
        request_id: requestId || undefined,
      })
      if (!result.success) {
        throw new Error(result.message || 'Failed to load favorite contexts')
      }
      return (
        (result.data as PaginatedResult<FavoriteConversationContext>) ??
        EMPTY_PAGINATED
      )
    },
    placeholderData: (previousData) => previousData,
  })
}

/**
 * Single conversation context detail, keyed by request id. The underlying
 * record lives in the context database (DB A) and may be removed when logs
 * are cleaned up.
 */
export function useConversationContextDetail(requestId: string) {
  return useQuery({
    queryKey: conversationContextKeys.detail(requestId),
    queryFn: async () => {
      const result = await getConversationContextDetail(requestId)
      if (!result.success) {
        throw new Error(result.message || 'Failed to load conversation context')
      }
      return result.data as ConversationContext | undefined
    },
    enabled: Boolean(requestId),
  })
}

/**
 * Single favorite conversation context detail, keyed by the favorite id in the
 * main database (DB B). Favorites are full snapshots and stay readable even
 * after the original context in DB A has been cleaned up.
 */
export function useFavoriteContextDetail(id: number) {
  return useQuery({
    queryKey: conversationContextKeys.favoriteDetail(id),
    queryFn: async () => {
      const result = await getFavoriteContextDetail(id)
      if (!result.success) {
        throw new Error(result.message || 'Failed to load favorite context')
      }
      return result.data as FavoriteConversationContext | undefined
    },
    enabled: Number.isFinite(id) && id > 0,
  })
}

/**
 * Mark a conversation context as favorite. Invalidate all conversation
 * context queries so lists and the detail page reflect the new state.
 */
export function useFavoriteConversationContext() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (requestId: string) => favoriteConversationContext(requestId),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: conversationContextKeys.all,
      })
    },
  })
}

/**
 * Remove a favorite snapshot. Favorites are only ever removed manually.
 */
export function useDeleteFavoriteContext() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deleteFavoriteContext(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: conversationContextKeys.all,
      })
    },
  })
}
