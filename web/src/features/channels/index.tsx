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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi, Link, useNavigate } from '@tanstack/react-router'
import { Settings2 } from 'lucide-react'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelOps } from './api'
import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'
import { CredentialProfilesPanel } from './credential-profiles/components/credential-profiles-panel'
import {
  type ChannelsSectionId,
  CHANNELS_DEFAULT_SECTION,
  CHANNELS_CREDENTIAL_PROFILES_SECTION,
  CHANNELS_SECTION_IDS,
} from './section-registry'

const route = getRouteApi('/_authenticated/channels/$section')

const CHANNELS_SECTION_META: Record<ChannelsSectionId, { titleKey: string }> = {
  list: {
    titleKey: 'Channels',
  },
  'credential-profiles': {
    titleKey: 'Credential Profiles',
  },
}

export function Channels() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const params = route.useParams()
  const activeSection = (params.section ??
    CHANNELS_DEFAULT_SECTION) as ChannelsSectionId

  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const currentUser = useAuthStore((s) => s.auth.user)
  const canSensitiveWrite = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )

  const channelOpsQuery = useQuery({
    queryKey: ['channel-ops'],
    queryFn: getChannelOps,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })
  const retryTimes = channelOpsQuery.data?.data?.retry_times
  const retryLabel =
    typeof retryTimes === 'number' ? `${t('Max Retries')}: ${retryTimes}` : null
  let retryBadge = null
  if (retryLabel) {
    retryBadge = isRoot ? (
      <Tooltip>
        <TooltipTrigger
          render={
            <Badge
              variant='outline'
              className='shrink-0 cursor-pointer'
              aria-label={t('Retry Settings')}
              render={
                <Link
                  to='/system-settings/models/$section'
                  params={{ section: 'routing-reliability' }}
                />
              }
            />
          }
        >
          <span>{retryLabel}</span>
          <Settings2 data-icon='inline-end' />
        </TooltipTrigger>
        <TooltipContent>
          <p>{t('Retry Settings')}</p>
        </TooltipContent>
      </Tooltip>
    ) : (
      <Badge variant='outline' className='shrink-0'>
        {retryLabel}
      </Badge>
    )
  }

  const handleSectionChange = useCallback(
    (section: string) => {
      void navigate({
        to: '/channels/$section',
        params: { section: section as ChannelsSectionId },
        search: true,
      })
    },
    [navigate]
  )

  const meta =
    CHANNELS_SECTION_META[activeSection] ?? CHANNELS_SECTION_META.list

  let sectionContent = null
  if (activeSection === 'list') {
    sectionContent = <ChannelsTable />
  } else if (canSensitiveWrite) {
    sectionContent = <CredentialProfilesPanel />
  }

  return (
    <ChannelsProvider>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          <span className='flex min-w-0 items-center gap-2'>
            <span className='truncate'>{t(meta.titleKey)}</span>
            {activeSection === 'list' ? retryBadge : null}
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {activeSection === 'list' ? <ChannelsPrimaryButtons /> : null}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4'>
            <Tabs value={activeSection} onValueChange={handleSectionChange}>
              <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
                {CHANNELS_SECTION_IDS.filter(
                  (section) =>
                    section !== CHANNELS_CREDENTIAL_PROFILES_SECTION ||
                    canSensitiveWrite
                ).map((section) => (
                  <TabsTrigger key={section} value={section}>
                    {t(CHANNELS_SECTION_META[section].titleKey)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
            <div className='min-h-0 flex-1'>{sectionContent}</div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ChannelsDialogs />
    </ChannelsProvider>
  )
}
