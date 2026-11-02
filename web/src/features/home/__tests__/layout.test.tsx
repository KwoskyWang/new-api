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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { PersonalizedHome } from '../personalized-home'

test('keeps the hero video behind the content without changing inline media positioning', async () => {
  const routeTree = createRootRoute({
    component: () => <PersonalizedHome isAuthenticated={false} />,
  })
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  const view = render(<RouterProvider router={router} />)
  await screen.findByRole('heading', { level: 1 })

  const background = view.container.querySelector(
    'video[src="/home/cloudSphere-4.mp4"]'
  )?.parentElement
  expect(background).toHaveClass('absolute', 'inset-0')
  expect(background).not.toHaveClass('relative')

  const inlineMedia = view.container.querySelector(
    'img[src="/home/gpt.jpg"]'
  )?.parentElement
  expect(inlineMedia).toHaveClass('relative')
  expect(inlineMedia).not.toHaveClass('absolute')
})
