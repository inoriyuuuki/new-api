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
import { createSectionRegistry } from '@/features/system-settings/utils/section-registry'

/**
 * Channels page section definitions
 */
const CHANNELS_SECTIONS = [
  {
    id: 'list',
    titleKey: 'Channels',
    build: () => null, // Content is rendered directly in the page component
  },
  {
    id: 'credential-profiles',
    titleKey: 'Credential Profiles',
    build: () => null, // Content is rendered directly in the page component
  },
] as const

export type ChannelsSectionId = (typeof CHANNELS_SECTIONS)[number]['id']

export const CHANNELS_CREDENTIAL_PROFILES_SECTION = 'credential-profiles'

const channelsRegistry = createSectionRegistry<
  ChannelsSectionId,
  Record<string, never>,
  []
>({
  sections: CHANNELS_SECTIONS,
  defaultSection: 'list',
  basePath: '/channels',
  urlStyle: 'path',
})

export const CHANNELS_SECTION_IDS = channelsRegistry.sectionIds
export const CHANNELS_DEFAULT_SECTION = channelsRegistry.defaultSection
export const getChannelsSectionNavItems = channelsRegistry.getSectionNavItems
