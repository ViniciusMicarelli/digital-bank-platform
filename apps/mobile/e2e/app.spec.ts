import { expect, test, type Page } from '@playwright/test'

const PASSWORD = 'correct horse battery staple'
const shots = process.env.SHOTS_DIR
const shot = async (page: Page, name: string) => { if (shots) await page.screenshot({ path: `${shots}/${name}.png` }) }

function randomCPF(): string {
  const d = [3 + Math.floor(Math.random() * 7), ...Array.from({ length: 8 }, () => Math.floor(Math.random() * 10))]
  for (const n of [9, 10]) {
    let sum = 0
    for (let i = 0; i < n; i++) sum += d[i] * (n + 1 - i)
    d.push(((sum * 10) % 11) % 10)
  }
  return d.join('')
}

test.beforeEach(({ page }) => { page.on('console', (m) => m.type() === 'error' && console.log('BROWSER:', m.text())); page.on('response', (r) => r.status() >= 400 && console.log('HTTP', r.status(), r.url())) })

test('a new customer onboards, receives and sends Pix, and uses the card', async ({ page }) => {
  const field = (label: string) => page.getByLabel(label, { exact: true }).filter({ visible: true }).first()
  const button = (name: string | RegExp) => page.getByRole('button', { name }).filter({ visible: true }).first()

  await page.goto('/')
  await page.getByText('Criar conta').filter({ visible: true }).first().click()
  await field('Nome completo').fill('Bruno Lima')
  await field('E-mail').fill(`mobile-${crypto.randomUUID().slice(0, 8)}@example.com`)
  await field('Senha').fill(PASSWORD)
  await button('Criar conta').click()

  await expect(page.getByText('Vamos abrir sua conta').filter({ visible: true }).first()).toBeVisible()
  await field('Nome completo').fill('Bruno Lima')
  await field('CPF').fill(randomCPF())
  await field('Data de nascimento').fill('20051990')
  await field('CEP').fill('01310100')
  await field('Rua').fill('Av. Paulista')
  await field('Número').fill('1000')
  await field('Cidade').fill('São Paulo')
  await shot(page, 'm01-onboarding')
  await button('Salvar e continuar').click()
  await button('Enviar para verificação').click()
  await button('Abrir minha conta').click()
  await expect(page.getByText('Saldo disponível').filter({ visible: true }).first()).toBeVisible()

  await page.getByRole('tab', { name: /Pix/ }).filter({ visible: true }).first().click()
  await page.getByRole('tab', { name: 'Receber' }).filter({ visible: true }).first().click()
  await field('Nova chave (e-mail)').fill(`bruno.${crypto.randomUUID().slice(0, 8)}@example.com`)
  await button('Cadastrar e-mail').click()
  await field('Valor (opcional)').fill('250')
  await button(/Receber R\$ 250,00/).click()
  await expect(page.getByText('Pix recebido').filter({ visible: true }).first()).toBeVisible() // live banner, over the XHR/fetch stream
  await button('Gerar QR Code').click()
  await expect(page.getByLabel('QR Code Pix').filter({ visible: true })).toBeVisible()
  await shot(page, 'm02-pix-receive')

  await page.getByRole('tab', { name: 'Enviar' }).filter({ visible: true }).first().click()
  await field('Chave Pix').fill('maria@outrobanco.com.br')
  await button('Continuar').click()
  await expect(page.getByText('Maria Oliveira').filter({ visible: true }).first()).toBeVisible()
  await field('Valor').fill('40')
  await button(/Enviar R\$ 40,00/).click()
  await expect(page.getByText('Pix enviado ✓').filter({ visible: true }).first()).toBeVisible()

  await page.getByRole('tab', { name: /Início/ }).filter({ visible: true }).first().click()
  await expect(page.getByLabel('Saldo').filter({ visible: true })).toHaveText('R$ 210,00')
  // The statement row (exact text), not the notification banner that mentions her.
  await expect(page.getByText('Maria Oliveira', { exact: true }).filter({ visible: true }).first()).toBeVisible()
  await shot(page, 'm03-home')

  await page.getByRole('tab', { name: /Cartões/ }).filter({ visible: true }).first().click()
  await button('Criar cartão virtual').click()
  await button('Ver dados').click()
  await field('Senha').fill(PASSWORD)
  await button('Confirmar').click()
  await expect(page.getByRole('dialog')).toHaveCount(0) // the sheet slides away first
  await expect(page.getByText('CVV').filter({ visible: true }).first()).toBeVisible()
  await field('Simular compra de').fill('15')
  await button('Passar o cartão').click()
  await expect(page.getByText('Aprovada: R$ 15,00').filter({ visible: true }).first()).toBeVisible()
  await shot(page, 'm04-cards')
})
