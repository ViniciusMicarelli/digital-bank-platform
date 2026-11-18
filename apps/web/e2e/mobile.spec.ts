import { expect, test } from '@playwright/test'
import { onboard, shot } from './helpers'

test('mobile layout: bottom navigation', async ({ page }) => {
  await onboard(page)
  const nav = page.locator('nav').last()
  await expect(nav.getByRole('link', { name: /Pix/ })).toBeVisible()
  await nav.getByRole('link', { name: /Extrato/ }).click()
  await expect(page.getByRole('heading', { name: 'Extrato' })).toBeVisible()
  await nav.getByRole('link', { name: /Mais/ }).click()
  await page.getByRole('link', { name: /Boletos/ }).click()
  await expect(page.getByRole('heading', { name: 'Boletos' })).toBeVisible()
  await shot(page, '09-mobile')
})
