import { createHmac } from 'node:crypto'
import { readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
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
  await page.waitForTimeout(1200) // entrance animation
  await shot(page, '00-register')
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

/** The analysts bootstrapped in docker compose, with their TOTP secrets. */
export const ANALYST = { email: 'analista@digitalbank.dev', secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP' }
export const REVIEWER = { email: 'revisora@digitalbank.dev', secret: 'KRSXG5CTMVRXEZLUKRSXG5CTMVRXEZLU' }

/** RFC 6238 code for a 30 s step, like an authenticator app. */
export function totp(secret: string, step: number): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret) bits += alphabet.indexOf(c).toString(2).padStart(5, '0')
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)))
  const msg = Buffer.alloc(8)
  msg.writeBigUInt64BE(BigInt(step))
  const mac = createHmac('sha1', key).update(msg).digest()
  const o = mac[mac.length - 1] & 0x0f
  return String((mac.readUInt32BE(o) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

/**
 * Signs an analyst in with password and TOTP. A code is accepted once and
 * refused codes count towards the account lockout, so the last step used per
 * analyst is kept in a file (shared by the light and dark runs) and a fresh
 * step is always used: this one, the next (the server allows one step of
 * drift), or the one after once it is close enough.
 */
export async function staffLogin(page: Page, who: { email: string; secret: string }) {
  const file = join(tmpdir(), `digital-bank-totp-${who.email}`)
  let last = 0
  try { last = Number(readFileSync(file, 'utf8')) || 0 } catch { /* first login */ }
  let now = Math.floor(Date.now() / 30_000)
  const step = Math.max(now, last + 1)
  if (step > now + 1) {
    await page.waitForTimeout((step - 1) * 30_000 - Date.now() + 500)
    now = step - 1
  }
  writeFileSync(file, String(step))
  await page.goto('/entrar')
  await page.getByLabel('E-mail').fill(who.email)
  await page.getByLabel('Senha').fill(PASSWORD)
  await page.getByRole('button', { name: 'Entrar' }).click()
  const code = page.getByLabel('Código do autenticador')
  await expect(code).toBeVisible()
  await code.fill(totp(who.secret, step))
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('heading', { name: 'Análises', exact: true })).toBeVisible()
}
