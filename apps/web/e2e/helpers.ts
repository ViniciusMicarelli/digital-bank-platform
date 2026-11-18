import { expect, type Page } from '@playwright/test'

export const PASSWORD = 'correct horse battery staple'
export const shots = process.env.SHOTS_DIR

export async function shot(page: Page, name: string) {
  if (shots) await page.screenshot({ path: `${shots}/${name}.png`, fullPage: true })
}

/** A valid CPF the bureau simulator approves (no 111/222 prefix). */
export function randomCPF(): string {
  const d = [3 + Math.floor(Math.random() * 7), ...Array.from({ length: 8 }, () => Math.floor(Math.random() * 10))]
  for (const n of [9, 10]) {
    let sum = 0
    for (let i = 0; i < n; i++) sum += d[i] * (n + 1 - i)
    d.push(((sum * 10) % 11) % 10)
  }
  return d.join('')
}

/** Signs up, completes onboarding (profile + KYC) and opens the account. */
export async function onboard(page: Page, name = 'Ana Souza') {
  const email = `web-${crypto.randomUUID().slice(0, 8)}@example.com`
  await page.goto('/criar-conta')
  await page.getByLabel('Nome completo').fill(name)
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(PASSWORD)
  await page.getByRole('button', { name: 'Criar conta' }).click()

  await expect(page.getByRole('heading', { name: 'Vamos abrir sua conta' })).toBeVisible()
  await page.getByLabel('Nome completo').fill(name)
  await page.getByLabel('CPF').fill(randomCPF())
  await page.getByLabel('Data de nascimento').fill('1990-05-20')
  await page.getByLabel('CEP').fill('01310100')
  await page.getByLabel('Rua').fill('Av. Paulista')
  await page.getByLabel('Número').fill('1000')
  await page.getByLabel('Cidade').fill('São Paulo')
  await page.getByRole('button', { name: 'Salvar e continuar' }).click()
  await page.getByRole('button', { name: 'Enviar para verificação' }).click()
  await expect(page.getByText('Identidade verificada')).toBeVisible()
  await page.getByRole('button', { name: 'Abrir minha conta' }).click()
  await expect(page.getByText('Saldo disponível')).toBeVisible()
  return email
}

/** Registers an e-mail Pix key and receives money from "another bank". */
export async function fund(page: Page, amount: string) {
  await page.goto('/pix?aba=chaves')
  const key = `ana.${crypto.randomUUID().slice(0, 8)}@example.com`
  await page.getByLabel('Chave', { exact: true }).fill(key)
  await page.getByRole('button', { name: 'Cadastrar' }).click()
  await expect(page.getByText(key).first()).toBeVisible()
  await page.getByLabel(/Receber um Pix de outro banco/).fill(amount)
  await page.getByRole('button', { name: 'Receber', exact: true }).click()
  await expect(page.getByText(/a caminho/)).toBeVisible()
  return key
}
