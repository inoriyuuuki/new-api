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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  buildCredentialProfileCreatePayload,
  buildCredentialProfileUpdatePayload,
  credentialProfileToFormState,
  createEmptyCredentialProfileForm,
  validateCredentialProfileForm,
  type CredentialProfileFormState,
} from './lib.ts'
import type { CredentialProfile } from './types'

const baseForm: CredentialProfileFormState = {
  name: 'OpenAI shared',
  updateKey: true,
  key: 'sk-test-123',
  updateBaseUrl: true,
  baseUrl: 'https://api.example.com',
  remark: 'team shared key',
}

describe('createEmptyCredentialProfileForm', () => {
  test('starts with a required key and no optional updates', () => {
    const form = createEmptyCredentialProfileForm()
    assert.equal(form.name, '')
    assert.equal(form.updateKey, true)
    assert.equal(form.key, '')
    assert.equal(form.updateBaseUrl, false)
    assert.equal(form.baseUrl, '')
    assert.equal(form.remark, '')
  })
})

describe('credentialProfileToFormState', () => {
  const profile: CredentialProfile = {
    id: 1,
    name: 'OpenAI shared',
    base_url: 'https://api.example.com',
    remark: 'team key',
    bound_count: 2,
    out_of_sync_count: 1,
    created_time: 100,
    updated_time: 200,
  }

  test('maps profile fields without exposing a key', () => {
    const form = credentialProfileToFormState(profile)
    assert.equal(form.name, 'OpenAI shared')
    assert.equal(form.baseUrl, 'https://api.example.com')
    assert.equal(form.remark, 'team key')
    assert.equal(form.key, '')
  })

  test('defaults optional updates to off so edit keeps stored values', () => {
    const form = credentialProfileToFormState(profile)
    assert.equal(form.updateKey, false)
    assert.equal(form.updateBaseUrl, false)
  })

  test('falls back to empty base URL when the profile has none', () => {
    const form = credentialProfileToFormState({ ...profile, base_url: null })
    assert.equal(form.baseUrl, '')
  })
})

describe('validateCredentialProfileForm', () => {
  test('accepts a complete create form', () => {
    assert.deepEqual(validateCredentialProfileForm(baseForm, 'create'), [])
  })

  test('requires a non-empty name', () => {
    const errors = validateCredentialProfileForm(
      { ...baseForm, name: '  ' },
      'create'
    )
    assert.ok(errors.includes('Profile name is required'))
  })

  test('requires a key on create', () => {
    const errors = validateCredentialProfileForm(
      { ...baseForm, key: ' \n ' },
      'create'
    )
    assert.ok(errors.includes('API key is required'))
  })

  test('does not require a key on edit when not updating it', () => {
    const errors = validateCredentialProfileForm(
      { ...baseForm, updateKey: false, key: '' },
      'edit'
    )
    assert.deepEqual(errors, [])
  })

  test('requires a key on edit when updating it', () => {
    const errors = validateCredentialProfileForm(
      { ...baseForm, key: '' },
      'edit'
    )
    assert.ok(errors.includes('API key is required'))
  })
})

describe('buildCredentialProfileCreatePayload', () => {
  test('sends key, base_url and remark', () => {
    const payload = buildCredentialProfileCreatePayload(baseForm)
    assert.deepEqual(payload, {
      name: 'OpenAI shared',
      key: 'sk-test-123',
      base_url: 'https://api.example.com',
      remark: 'team shared key',
    })
  })

  test('omits empty base_url and remark', () => {
    const payload = buildCredentialProfileCreatePayload({
      ...baseForm,
      baseUrl: '  ',
      remark: '',
    })
    assert.deepEqual(payload, {
      name: 'OpenAI shared',
      key: 'sk-test-123',
    })
  })

  test('trims name and remark', () => {
    const payload = buildCredentialProfileCreatePayload({
      ...baseForm,
      name: '  shared  ',
      remark: '  team  ',
    })
    assert.equal(payload.name, 'shared')
    assert.equal(payload.remark, 'team')
  })
})

describe('buildCredentialProfileUpdatePayload', () => {
  test('omits key and base_url unless opted in', () => {
    const payload = buildCredentialProfileUpdatePayload({
      ...baseForm,
      updateKey: false,
      updateBaseUrl: false,
    })
    assert.deepEqual(payload, {
      name: 'OpenAI shared',
      remark: 'team shared key',
    })
  })

  test('sends key only when updating it', () => {
    const payload = buildCredentialProfileUpdatePayload({
      ...baseForm,
      updateBaseUrl: false,
    })
    assert.deepEqual(payload, {
      name: 'OpenAI shared',
      key: 'sk-test-123',
      remark: 'team shared key',
    })
  })

  test('sends an explicit empty base_url to clear it', () => {
    const payload = buildCredentialProfileUpdatePayload({
      ...baseForm,
      updateKey: false,
      baseUrl: '',
    })
    assert.equal(payload.base_url, '')
  })
})
