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
 * Best-effort extraction of the human-readable message list from a captured
 * conversation context.
 *
 * Supported request shapes:
 * - OpenAI chat completions: `messages[]`
 * - OpenAI Responses: `input[]` (+ optional `instructions`)
 * - Anthropic Messages: `messages[]` (+ optional top-level `system`)
 * - Gemini generateContent: `contents[]` (+ optional `systemInstruction`)
 *
 * Supported response shapes:
 * - OpenAI chat completions: `choices[0].message`
 * - OpenAI Responses: `output[]` / `output_text`
 * - Anthropic Messages: `content[]`
 * - Gemini generateContent: `candidates[0].content`
 * - SSE streams (OpenAI chat chunks, OpenAI Responses events,
 *   Anthropic events, Gemini chunks) with best-effort delta aggregation.
 *
 * Everything here is a draft and intentionally lossy: when a payload cannot
 * be recognized the extractor returns `null` so the UI can hide the message
 * area gracefully while the raw request/response bodies stay fully visible.
 */
export interface ConversationMessage {
  /** 1-based sequence number used as the accordion item value. */
  index: number
  /** Raw role coming from the provider payload (user/assistant/model/...). */
  role: string
  /** Extracted human-readable text. */
  content: string
  /** One-line truncated preview shown while the message is collapsed. */
  preview: string
  /** Number of characters in `content`. */
  charCount: number
  /** Where the message came from. */
  source: 'request' | 'response'
}

export interface ConversationMessagesResult {
  messages: ConversationMessage[]
  /**
   * True when the request body looked like a conversation payload but no
   * message could be extracted from it (useful for diagnostics).
   */
  parsedRequest: boolean
  /** True when at least one response message could be extracted. */
  parsedResponse: boolean
}

type JsonObject = Record<string, unknown>

const PREVIEW_MAX = 140

function tryParseJson(value: string): JsonObject | null {
  if (!value) return null
  const trimmed = value.trim()
  if (!trimmed) return null
  try {
    const parsed: unknown = JSON.parse(trimmed)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as JsonObject
    }
    return null
  } catch {
    return null
  }
}

function toText(part: unknown): string {
  if (typeof part === 'string') return part
  if (!part || typeof part !== 'object') return ''
  const obj = part as JsonObject

  // OpenAI Responses content parts carry a `type` plus `text`.
  switch (obj.type) {
    case 'text':
    case 'input_text':
    case 'output_text':
    case 'summary_text':
      return typeof obj.text === 'string' ? obj.text : ''
    case 'image':
    case 'image_url':
    case 'input_image':
      return '[image]'
    case 'audio':
    case 'input_audio':
      return '[audio]'
    case 'tool_use':
      return typeof obj.name === 'string'
        ? `[tool_use: ${obj.name}]`
        : '[tool_use]'
    case 'tool_result':
      return contentToText(obj.content)
    case 'function_call':
      return typeof obj.name === 'string'
        ? `[function_call: ${obj.name}]`
        : '[function_call]'
    case 'function_call_output':
      return typeof obj.output === 'string'
        ? obj.output
        : '[function_call_output]'
    default:
      break
  }

  // Gemini-style parts: `{ text }`, `{ inlineData }`, `{ functionCall }`,
  // `{ functionResponse }` (no `type` field).
  if (typeof obj.text === 'string') return obj.text
  if (obj.inlineData != null) return '[image]'
  if (obj.functionCall != null) {
    const fn = obj.functionCall as JsonObject
    return typeof fn.name === 'string'
      ? `[function_call: ${fn.name}]`
      : '[function_call]'
  }
  if (obj.functionResponse != null) {
    const fn = obj.functionResponse as JsonObject
    return contentToText(fn.response)
  }

  try {
    return JSON.stringify(obj)
  } catch {
    return ''
  }
}

function contentToText(content: unknown): string {
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content
      .map((part) => toText(part).trim())
      .filter(Boolean)
      .join('\n')
  }
  if (content && typeof content === 'object') {
    const obj = content as JsonObject
    // OpenAI Responses inline content object.
    if (typeof obj.text === 'string') return obj.text
  }
  return ''
}

function buildMessage(
  role: string,
  content: string,
  source: 'request' | 'response',
  index: number
): ConversationMessage {
  const text = content.trim()
  const oneLine = text.replaceAll(/\s+/g, ' ').trim()
  const preview =
    oneLine.length > PREVIEW_MAX ? `${oneLine.slice(0, PREVIEW_MAX)}…` : oneLine
  return {
    index,
    role: role || 'message',
    content: text,
    preview,
    charCount: text.length,
    source,
  }
}

function roleOf(item: JsonObject, fallback = ''): string {
  const role = item.role
  if (typeof role === 'string' && role.trim()) return role.trim()
  const type = item.type
  if (typeof type === 'string' && type.trim()) {
    if (type === 'message') return 'assistant'
    if (type === 'function_call' || type === 'function_call_output') {
      return 'tool'
    }
  }
  return fallback
}

function extractFromOpenAiChatRequest(json: JsonObject): ConversationMessage[] {
  const messages: ConversationMessage[] = []
  // Anthropic places the system prompt at the top level; it can be a plain
  // string or an array of content blocks.
  if (json.system != null) {
    const systemContent = contentToText(json.system)
    if (systemContent) {
      messages.push(
        buildMessage('system', systemContent, 'request', messages.length + 1)
      )
    }
  }
  const list = json.messages
  if (!Array.isArray(list)) return messages
  for (const item of list) {
    if (!item || typeof item !== 'object') continue
    const obj = item as JsonObject
    const content = contentToText(obj.content)
    const role = roleOf(obj)
    if (!content && !role) continue
    messages.push(
      buildMessage(
        role || 'message',
        content || '',
        'request',
        messages.length + 1
      )
    )
  }
  return messages
}

function extractFromResponsesRequest(json: JsonObject): ConversationMessage[] {
  const messages: ConversationMessage[] = []
  if (typeof json.instructions === 'string' && json.instructions.trim()) {
    messages.push(
      buildMessage('system', json.instructions, 'request', messages.length + 1)
    )
  }
  const list = json.input
  if (!Array.isArray(list)) return messages
  for (const item of list) {
    if (!item || typeof item !== 'object') continue
    const obj = item as JsonObject
    const type = typeof obj.type === 'string' ? obj.type : ''
    if (type === 'function_call' || type === 'function_call_output') {
      let name = 'tool'
      if (typeof obj.name === 'string') name = obj.name
      else if (typeof obj.call_id === 'string') name = obj.call_id
      const output = typeof obj.output === 'string' ? obj.output : ''
      const args = typeof obj.arguments === 'string' ? obj.arguments : ''
      messages.push(
        buildMessage(
          'tool',
          output || `[${type}: ${name}]${args ? ` ${args}` : ''}`,
          'request',
          messages.length + 1
        )
      )
      continue
    }
    const content = contentToText(obj.content)
    const role = roleOf(obj)
    if (!content && !role) continue
    messages.push(
      buildMessage(
        role || 'message',
        content || '',
        'request',
        messages.length + 1
      )
    )
  }
  return messages
}

function extractFromGeminiRequest(json: JsonObject): ConversationMessage[] {
  const messages: ConversationMessage[] = []
  const system = json.systemInstruction
  if (system && typeof system === 'object') {
    const obj = system as JsonObject
    const content = contentToText(obj.parts ?? obj.content)
    if (content) {
      messages.push(
        buildMessage('system', content, 'request', messages.length + 1)
      )
    }
  }
  const list = json.contents
  if (!Array.isArray(list)) return messages
  for (const item of list) {
    if (!item || typeof item !== 'object') continue
    const obj = item as JsonObject
    const content = contentToText(obj.parts ?? obj.content)
    if (!content) continue
    messages.push(
      buildMessage(roleOf(obj, 'user'), content, 'request', messages.length + 1)
    )
  }
  return messages
}

/**
 * Extract the ordered message list from a captured request body. Returns
 * `null` when the body does not look like a conversation payload at all.
 */
export function extractRequestMessages(
  requestBody: string
): ConversationMessage[] | null {
  const json = tryParseJson(requestBody)
  if (!json) return null

  if (Array.isArray(json.messages)) {
    const messages = extractFromOpenAiChatRequest(json)
    return messages.length > 0 ? messages : null
  }
  if (Array.isArray(json.input)) {
    const messages = extractFromResponsesRequest(json)
    return messages.length > 0 ? messages : null
  }
  // OpenAI Responses also accepts the input as a plain string prompt.
  if (typeof json.input === 'string' && json.input.trim()) {
    return [buildMessage('user', json.input, 'request', 1)]
  }
  if (Array.isArray(json.contents)) {
    const messages = extractFromGeminiRequest(json)
    return messages.length > 0 ? messages : null
  }
  return null
}

/**
 * OpenAI Responses output: the final reply is a single assistant/model
 * message. Reasoning summaries, tool/function calls and their outputs are not
 * appended as separate final-reply entries; only the last output item that
 * carries assistant/model text is used.
 */
function extractFinalResponseMessage(
  output: unknown
): ConversationMessage | null {
  if (!Array.isArray(output)) return null
  let last: ConversationMessage | null = null
  for (const item of output) {
    if (!item || typeof item !== 'object') continue
    const obj = item as JsonObject
    const type = typeof obj.type === 'string' ? obj.type : ''
    if (
      type === 'reasoning' ||
      type === 'function_call' ||
      type === 'function_call_output'
    ) {
      continue
    }
    const role = roleOf(obj)
    if (role !== 'assistant' && role !== 'model') continue
    const content = contentToText(obj.content)
    if (!content) continue
    last = buildMessage(role, content, 'response', 1)
  }
  return last
}

function extractFromResponseJson(
  json: JsonObject
): ConversationMessage[] | null {
  // OpenAI Responses non-stream output: only the final reply is appended.
  if (Array.isArray(json.output)) {
    const finalMessage = extractFinalResponseMessage(json.output)
    if (finalMessage) return [finalMessage]
  }
  if (typeof json.output_text === 'string' && json.output_text.trim()) {
    return [buildMessage('assistant', json.output_text, 'response', 1)]
  }

  // OpenAI chat completions.
  if (Array.isArray(json.choices) && json.choices.length > 0) {
    const choice = json.choices[0]
    if (choice && typeof choice === 'object') {
      const message = (choice as JsonObject).message
      if (message && typeof message === 'object') {
        const obj = message as JsonObject
        const content = contentToText(obj.content)
        const reasoning =
          typeof obj.reasoning_content === 'string' ? obj.reasoning_content : ''
        const combined =
          reasoning && content
            ? `${reasoning}\n\n${content}`
            : reasoning || content
        if (combined.trim()) {
          return [buildMessage('assistant', combined, 'response', 1)]
        }
      }
    }
  }

  // Anthropic Messages non-stream content.
  if (Array.isArray(json.content)) {
    const content = contentToText(json.content)
    if (content) return [buildMessage('assistant', content, 'response', 1)]
  }

  // Gemini generateContent.
  if (Array.isArray(json.candidates) && json.candidates.length > 0) {
    const candidate = json.candidates[0]
    if (candidate && typeof candidate === 'object') {
      const obj = candidate as JsonObject
      const contentObj = obj.content
      const content = contentToText(
        contentObj && typeof contentObj === 'object'
          ? ((contentObj as JsonObject).parts ?? contentObj)
          : contentObj
      )
      if (content) return [buildMessage('model', content, 'response', 1)]
    }
  }

  return null
}

/** Aggregate OpenAI chat SSE chunks (content + reasoning deltas). */
function aggregateOpenAiChatChunks(body: string): string | null {
  const chunks: string[] = []
  for (const line of body.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('data:')) continue
    const payload = trimmed.slice(5).trim()
    if (!payload || payload === '[DONE]') continue
    let json: unknown
    try {
      json = JSON.parse(payload)
    } catch {
      continue
    }
    if (!json || typeof json !== 'object') continue
    const obj = json as JsonObject
    const choices = obj.choices
    if (!Array.isArray(choices) || choices.length === 0) continue
    const choice = choices[0]
    const delta =
      choice && typeof choice === 'object'
        ? (choice as JsonObject).delta
        : undefined
    if (!delta || typeof delta !== 'object') continue
    const d = delta as JsonObject
    if (typeof d.content === 'string' && d.content) chunks.push(d.content)
    else if (typeof d.reasoning_content === 'string' && d.reasoning_content) {
      chunks.push(d.reasoning_content)
    }
  }
  const joined = chunks.join('').trim()
  return joined || null
}

/** Aggregate Anthropic SSE events into one assistant message. */
function aggregateAnthropicSse(body: string): string | null {
  const chunks: string[] = []
  for (const line of body.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('data:')) continue
    const payload = trimmed.slice(5).trim()
    if (!payload || payload === '[DONE]') continue
    let json: unknown
    try {
      json = JSON.parse(payload)
    } catch {
      continue
    }
    if (!json || typeof json !== 'object') continue
    const obj = json as JsonObject
    const type = typeof obj.type === 'string' ? obj.type : ''
    if (type === 'content_block_delta') {
      const delta = obj.delta
      if (
        delta &&
        typeof delta === 'object' &&
        typeof (delta as JsonObject).text === 'string'
      ) {
        chunks.push((delta as JsonObject).text as string)
      }
      continue
    }
    if (type === 'message_start') {
      const message = obj.message
      if (message && typeof message === 'object') {
        const content = contentToText((message as JsonObject).content)
        if (content) chunks.push(content)
      }
      continue
    }
    if (type === 'message_delta') {
      // Ignore stop_reason deltas without text.
      continue
    }
  }
  const joined = chunks.join('').trim()
  return joined || null
}

/** Aggregate Gemini SSE chunks into one model message. */
function aggregateGeminiSse(body: string): string | null {
  const chunks: string[] = []
  for (const line of body.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('data:')) continue
    const payload = trimmed.slice(5).trim()
    if (!payload || payload === '[DONE]') continue
    let json: unknown
    try {
      json = JSON.parse(payload)
    } catch {
      continue
    }
    if (!json || typeof json !== 'object') continue
    const obj = json as JsonObject
    const candidates = obj.candidates
    if (!Array.isArray(candidates) || candidates.length === 0) continue
    const candidate = candidates[0]
    if (!candidate || typeof candidate !== 'object') continue
    const content = (candidate as JsonObject).content
    if (!content || typeof content !== 'object') continue
    const parts = (content as JsonObject).parts
    if (!Array.isArray(parts)) continue
    for (const part of parts) {
      if (
        part &&
        typeof part === 'object' &&
        typeof (part as JsonObject).text === 'string'
      ) {
        const text = (part as JsonObject).text as string
        if (text) chunks.push(text)
      }
    }
  }
  const joined = chunks.join('').trim()
  return joined || null
}

function isSseBody(body: string): boolean {
  return body.includes('\ndata:') || body.startsWith('data:')
}

/**
 * Extract the final reply (or reply messages) from a captured response body.
 * Returns `null` when nothing could be recognized so the UI can hide itself.
 */
export function extractResponseMessages(
  responseBody: string
): ConversationMessage[] | null {
  if (!responseBody || !responseBody.trim()) return null

  if (isSseBody(responseBody)) {
    // OpenAI Responses SSE carries a final `response.completed` event with the
    // full response object; prefer it over delta aggregation when available.
    for (const line of responseBody.split(/\r?\n/)) {
      const trimmed = line.trim()
      if (!trimmed.startsWith('data:')) continue
      const payload = trimmed.slice(5).trim()
      if (!payload || payload === '[DONE]') continue
      let json: unknown
      try {
        json = JSON.parse(payload)
      } catch {
        continue
      }
      if (!json || typeof json !== 'object') continue
      const obj = json as JsonObject
      if (
        obj.type === 'response.completed' &&
        obj.response &&
        typeof obj.response === 'object'
      ) {
        const messages = extractFromResponseJson(obj.response as JsonObject)
        if (messages && messages.length > 0) return messages
      }
    }

    const openAi = aggregateOpenAiChatChunks(responseBody)
    if (openAi) return [buildMessage('assistant', openAi, 'response', 1)]
    const anthropic = aggregateAnthropicSse(responseBody)
    if (anthropic) return [buildMessage('assistant', anthropic, 'response', 1)]
    const gemini = aggregateGeminiSse(responseBody)
    if (gemini) return [buildMessage('model', gemini, 'response', 1)]
    return null
  }

  const json = tryParseJson(responseBody)
  if (!json) return null
  return extractFromResponseJson(json)
}

/**
 * Build the full ordered message list shown in the detail page: request
 * messages first, then the final reply as the last entry.
 */
export function buildConversationMessages(
  requestBody: string | undefined,
  responseBody: string | undefined
): ConversationMessagesResult {
  const requestMessages = extractRequestMessages(requestBody ?? '')
  const responseMessages = extractResponseMessages(responseBody ?? '')
  const messages: ConversationMessage[] = []
  if (requestMessages) messages.push(...requestMessages)
  if (responseMessages) {
    for (const message of responseMessages) {
      messages.push({ ...message, index: messages.length + 1 })
    }
  }
  return {
    messages,
    parsedRequest: requestMessages != null,
    parsedResponse: responseMessages != null,
  }
}

/** Index (accordion value) of the last user-role request message, if any. */
export function findLastUserMessageIndex(
  messages: ConversationMessage[]
): number | null {
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    if (messages[i].source === 'request' && messages[i].role === 'user') {
      return messages[i].index
    }
  }
  return null
}
