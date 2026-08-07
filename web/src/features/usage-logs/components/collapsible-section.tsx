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
import { ChevronDown } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

/**
 * Collapsible card section used by the conversation context detail page: the
 * title row stays visible and a toggle on the right expands/collapses the
 * body. Request metadata stays expanded by default; every other area
 * collapses by default.
 */
export function CollapsibleSection({
  title,
  defaultExpanded = false,
  badge,
  actions,
  contentClassName,
  children,
}: {
  title: string
  defaultExpanded?: boolean
  badge?: React.ReactNode
  actions?: React.ReactNode
  contentClassName?: string
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(defaultExpanded)

  return (
    <Card>
      <Collapsible open={expanded} onOpenChange={setExpanded}>
        <CardHeader className='flex-row items-center justify-between gap-2 border-b'>
          <div className='flex min-w-0 items-center gap-2'>
            <CardTitle className='text-sm font-semibold'>{title}</CardTitle>
            {badge}
          </div>
          <div className='flex shrink-0 items-center gap-1'>
            {actions}
            <CollapsibleTrigger
              className='text-muted-foreground hover:text-foreground inline-flex h-7 items-center gap-1 rounded-md px-2 text-xs font-medium transition-colors'
              aria-label={expanded ? t('Collapse') : t('Expand')}
            >
              <ChevronDown
                className={cn(
                  'size-3.5 transition-transform',
                  expanded && 'rotate-180'
                )}
              />
              {expanded ? t('Collapse') : t('Expand')}
            </CollapsibleTrigger>
          </div>
        </CardHeader>
        <CollapsibleContent>
          <CardContent className={contentClassName}>{children}</CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  )
}
