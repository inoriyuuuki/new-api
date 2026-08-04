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
import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { ConversationContextDetail } from '@/features/usage-logs/components/conversation-context-detail'

export const Route = createFileRoute(
  '/_authenticated/usage-logs/favorite-context/$id'
)({
  component: FavoriteContextRoute,
})

function FavoriteContextRoute() {
  const { t } = useTranslation()
  const { id } = Route.useParams()
  const favoriteId = Number(id)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Conversation Context')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          className='gap-1.5'
          render={
            <Link
              to='/usage-logs/$section'
              params={{ section: 'favorite-contexts' }}
            />
          }
        >
          <ArrowLeft className='size-3.5' />
          {t('Back to contexts')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <ConversationContextDetail favoriteId={favoriteId} />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
