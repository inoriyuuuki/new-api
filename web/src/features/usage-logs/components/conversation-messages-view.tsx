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
import type { VariantProps } from 'class-variance-authority'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

import {
  buildConversationMessages,
  findLastUserMessageIndex,
  type ConversationMessage,
} from '../lib/conversation-messages'

const ROLE_LABEL_KEYS: Record<string, string> = {
  system: 'System',
  user: 'User',
  assistant: 'Assistant',
  model: 'Model',
  tool: 'Tool',
  function: 'Function',
  developer: 'System',
}

function roleBadgeVariant(
  role: string
): VariantProps<typeof badgeVariants>['variant'] {
  switch (role) {
    case 'user':
      return 'default'
    case 'assistant':
    case 'model':
      return 'secondary'
    case 'system':
    case 'developer':
      return 'outline'
    case 'tool':
    case 'function':
      return 'destructive'
    default:
      return 'ghost'
  }
}

function MessageRoleBadge({ role }: { role: string }) {
  const { t } = useTranslation()
  const labelKey = ROLE_LABEL_KEYS[role]
  const label = labelKey ? t(labelKey) : role
  return <Badge variant={roleBadgeVariant(role)}>{label}</Badge>
}

/**
 * Draft visualization of the conversation messages captured for a usage log.
 *
 * Request messages are listed in order and the final reply from the response
 * is appended as the last entry. Every entry is collapsible; by default the
 * last user message and the final reply start expanded so the meaningful tail
 * of the conversation is immediately visible.
 *
 * The extractor is best-effort: when nothing can be parsed the whole area
 * hides itself and the raw Request/Response body blocks below remain the
 * source of truth.
 */
export function ConversationMessagesView({
  requestBody,
  responseBody,
}: {
  requestBody: string | undefined
  responseBody: string | undefined
}) {
  const { t } = useTranslation()
  const result = useMemo(
    () => buildConversationMessages(requestBody, responseBody),
    [requestBody, responseBody]
  )

  const messages = result.messages
  const defaultOpenValues = useMemo(() => {
    const values: string[] = []
    const lastUser = findLastUserMessageIndex(messages)
    if (lastUser != null) values.push(String(lastUser))
    const lastMessage = messages.at(-1)
    if (lastMessage) values.push(String(lastMessage.index))
    return values
  }, [messages])

  // Nothing recognized: hide the draft area entirely so the raw bodies stay
  // the single source of truth without an empty panel.
  if (messages.length === 0) return null

  return (
    <Card>
      <CardHeader className='flex-row items-center justify-between gap-2 border-b'>
        <CardTitle className='text-sm font-semibold'>
          {t('Message List')}
        </CardTitle>
        <Badge variant='secondary'>
          {messages.length} {t('Messages')}
        </Badge>
      </CardHeader>
      <CardContent className='px-0 py-0'>
        <Accordion multiple defaultValue={defaultOpenValues}>
          {messages.map((message) => (
            <ConversationMessageItem key={message.index} message={message} />
          ))}
        </Accordion>
      </CardContent>
    </Card>
  )
}

function ConversationMessageItem({
  message,
}: {
  message: ConversationMessage
}) {
  const { t } = useTranslation()
  const isFinalReply = message.source === 'response'

  return (
    <AccordionItem value={String(message.index)}>
      <AccordionTrigger className='gap-2 px-4'>
        <span className='flex min-w-0 flex-1 items-center gap-2'>
          <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
            #{message.index}
          </span>
          <MessageRoleBadge role={message.role} />
          {isFinalReply && (
            <Badge variant='outline' className='text-muted-foreground'>
              {t('Final Response')}
            </Badge>
          )}
          <span
            className='text-muted-foreground min-w-0 truncate text-xs'
            title={message.preview}
          >
            {message.preview || t('Empty')}
          </span>
        </span>
        <span className='text-muted-foreground hidden shrink-0 text-xs tabular-nums sm:inline'>
          {message.charCount.toLocaleString()} {t('characters')}
        </span>
      </AccordionTrigger>
      <AccordionContent>
        <pre
          className={cn(
            'text-muted-foreground mx-4 max-h-[360px] overflow-auto rounded-md bg-muted/40 px-3 py-2.5',
            'font-mono text-[11px] leading-relaxed break-all whitespace-pre-wrap'
          )}
        >
          {message.content || t('Empty')}
        </pre>
      </AccordionContent>
    </AccordionItem>
  )
}
