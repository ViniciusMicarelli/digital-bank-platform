import { expect, test } from '@playwright/test'
import { fund, onboard, PASSWORD, shot } from './helpers'

test('onboarding, Pix in and out with live notifications', async ({ page }) => {
  await onboard(page)
  await shot(page, '01-home-empty')
  await fund(page, '500,00')
  // The notification arrives over the live stream and refreshes the balance.
  await expect(page.getByText('Pix recebido').first()).toBeVisible()
  await page.goto('/')
  await expect(page.getByText('R$ 500,00').first()).toBeVisible()

  await page.goto('/pix?aba=enviar')
  await page.getByLabel('Chave Pix').fill('maria@outrobanco.com.br')
  await page.getByRole('button', { name: 'Continuar' }).click()
  await expect(page.getByText('Maria Oliveira')).toBeVisible()
  await page.getByLabel('Valor').fill('120,50')
  await shot(page, '02-pix-send')
  await page.getByRole('button', { name: /^Enviar R\$ 120,50/ }).click()
  await expect(page.getByText('Pix enviado').first()).toBeVisible()

  await page.goto('/')
  await expect(page.getByText('R$ 379,50').first()).toBeVisible()
  // The statement row (exact text), not the toast that mentions her.
  await expect(page.getByText('Maria Oliveira', { exact: true }).first()).toBeVisible()
  await shot(page, '03-home')

  await page.goto('/pix?aba=cobrar')
  await page.getByLabel('Valor (opcional)').fill('25')
  await page.getByRole('button', { name: 'Gerar QR Code' }).click()
  await expect(page.getByAltText('QR Code Pix')).toBeVisible()
  await shot(page, '04-pix-qr')
})

test('refusals are explained', async ({ page }) => {
  await onboard(page)
  await page.goto('/pix?aba=enviar')
  await page.getByLabel('Chave Pix').fill('maria@outrobanco.com.br')
  await page.getByRole('button', { name: 'Continuar' }).click()
  await page.getByLabel('Valor').fill('10')
  await page.getByRole('button', { name: /^Enviar/ }).click()
  await expect(page.getByRole('alert')).toHaveText('Saldo insuficiente.')
})

test('cards: step-up to reveal, purchase, wallet', async ({ page }) => {
  await onboard(page)
  await fund(page, '200,00')
  await page.goto('/cartoes')
  await page.getByRole('button', { name: 'Criar cartão virtual' }).click()
  await page.getByRole('button', { name: 'Ver dados' }).click()
  await expect(page.getByText('Confirme que é você')).toBeVisible()
  await page.getByLabel('Senha').fill(PASSWORD)
  await page.getByRole('button', { name: 'Confirmar' }).click()
  await expect(page.getByText('CVV')).toBeVisible()

  await page.getByLabel('Simular compra de').fill('42')
  await page.getByRole('button', { name: 'Passar o cartão' }).click()
  await expect(page.getByText(/Aprovada: R\$ 42,00/)).toBeVisible()
  await expect(page.getByText('Compra aprovada').first()).toBeVisible() // live notification

  await page.getByRole('button', { name: 'Adicionar' }).click()
  await page.getByLabel('Aparelho').fill('iPhone da Ana')
  await page.getByRole('dialog').getByRole('button', { name: 'Adicionar' }).click()
  await expect(page.getByText(/Cartão adicionado ao Apple Pay/)).toBeVisible()
  await page.getByRole('button', { name: 'Fechar' }).click()
  await expect(page.getByText('Apple Pay · iPhone da Ana')).toBeVisible()
  await shot(page, '05-cards')
})

test('credit invoice and boleto', async ({ page }) => {
  await onboard(page)
  await fund(page, '300,00')
  await page.goto('/credito')
  await page.getByRole('button', { name: 'Ativar crédito' }).click()
  await expect(page.getByText('R$ 1.000,00').first()).toBeVisible()
  await page.getByRole('button', { name: 'Fechar a fatura agora' }).click()
  await expect(page.getByText('Faturas')).toBeVisible()
  await shot(page, '06-credit')

  await page.goto('/boletos')
  await page.getByLabel('Gerar boleto de').fill('89,90')
  await page.getByRole('button', { name: 'Gerar e consultar' }).click()
  await expect(page.getByText('CIA PAULISTA DE ENERGIA')).toBeVisible()
  await shot(page, '07-boleto-quote')
  await page.getByRole('button', { name: 'Pagar agora' }).click()
  await expect(page.getByText('Boleto pago').first()).toBeVisible()

  await page.goto('/notificacoes')
  await expect(page.getByText('Boleto pago').first()).toBeVisible()
  await expect(page.getByText('Cadastro aprovado')).toBeVisible()
  await shot(page, '08-notifications')
})
