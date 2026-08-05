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
/**
 * Zod schemas for common logs
 * This file should only contain Zod schemas and types inferred from them
 */
import { z } from 'zod'

// Usage log schema
export const usageLogSchema = z.object({
  id: z.number(),
  user_id: z.number(),
  created_at: z.number(),
  type: z.number(),
  content: z.string(),
  username: z.string().default(''),
  token_name: z.string().default(''),
  model_name: z.string().default(''),
  quota: z.number().default(0),
  prompt_tokens: z.number().default(0),
  completion_tokens: z.number().default(0),
  use_time: z.number().default(0),
  is_stream: z.boolean().default(false),
  channel: z.number().default(0),
  channel_name: z.string().nullish().default(''),
  token_id: z.number().default(0),
  group: z.string().default(''),
  ip: z.string().default(''),
  other: z.string().default(''),
  request_id: z.string().default(''),
  upstream_request_id: z.string().default(''),
  // Whether a conversation context record exists for this log entry. The
  // backend fills this in when a captured request/response is available;
  // entries without a context must not show the "view context" entry.
  has_context: z.boolean().default(false),
})

export type UsageLog = z.infer<typeof usageLogSchema>

// ============================================================================
// Conversation context schemas
// ============================================================================

/**
 * Captured request/response context linked to a usage log entry.
 * Stored in a separate log database (DB A) to keep the main database small.
 */
export const conversationContextSchema = z.object({
  id: z.number().default(0),
  log_id: z.number().default(0),
  request_id: z.string().default(''),
  user_id: z.number().default(0),
  created_at: z.number().default(0),
  request_path: z.string().default(''),
  relay_format: z.string().default(''),
  model_name: z.string().default(''),
  request_body: z.string().default(''),
  response_body: z.string().default(''),
  response_status: z.number().default(0),
  is_stream: z.boolean().default(false),
  capture_status: z.string().default(''),
  is_favorite: z.boolean().default(false),
})

export type ConversationContext = z.infer<typeof conversationContextSchema>

/**
 * Favorite snapshot of a conversation context. Lives in the main database
 * (DB B) so it survives context/log cleanup and is only removed manually.
 */
export const favoriteConversationContextSchema =
  conversationContextSchema.extend({
    favorited_at: z.number().default(0),
    source_user_id: z.number().default(0),
  })

export type FavoriteConversationContext = z.infer<
  typeof favoriteConversationContextSchema
>
