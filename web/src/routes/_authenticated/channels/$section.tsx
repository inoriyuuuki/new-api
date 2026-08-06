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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { Channels } from '@/features/channels'
import {
  CHANNELS_CREDENTIAL_PROFILES_SECTION,
  CHANNELS_DEFAULT_SECTION,
  CHANNELS_SECTION_IDS,
} from '@/features/channels/section-registry'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const channelsSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(undefined),
  filter: z.string().optional().catch(''),
  status: z.array(z.string()).optional().catch([]),
  type: z.array(z.string()).optional().catch([]),
  group: z.array(z.string()).optional().catch([]),
  model: z.string().optional().catch(''),
})

export const Route = createFileRoute('/_authenticated/channels/$section')({
  beforeLoad: ({ params }) => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }

    const validSections = CHANNELS_SECTION_IDS as unknown as string[]
    if (!validSections.includes(params.section)) {
      throw redirect({
        to: '/channels/$section',
        params: { section: CHANNELS_DEFAULT_SECTION },
        search: true,
      })
    }

    // The credential profiles page performs sensitive writes (API keys/base
    // URLs). Safely degrade to the channel list when the action is not granted.
    if (
      params.section === CHANNELS_CREDENTIAL_PROFILES_SECTION &&
      !hasPermission(
        auth.user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      )
    ) {
      throw redirect({
        to: '/channels/$section',
        params: { section: CHANNELS_DEFAULT_SECTION },
        search: true,
      })
    }
  },
  validateSearch: channelsSearchSchema,
  component: Channels,
})
