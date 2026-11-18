import { expect, test } from '@playwright/test'
import { PASSWORD, randomCPF, shot } from './helpers'

const api = (path: string) => `http://127.0.0.1:8000${path}`

test('an analyst approves a manual KYC review from the queue', async ({ page, request }) => {
  // A politically exposed customer (CPF prefix 222) waits for an analyst.
  const tag = crypto.randomUUID().slice(0, 6)
  const name = `Carla ${tag.replace(/[0-9]/g, 'x')} Dias`
  const email = `pep-${tag}@example.com`
  await request.post(api('/v1/auth/register'), { data: { email, password: PASSWORD, full_name: name } })
  const login = await (await request.post(api('/v1/auth/login'), { data: { email, password: PASSWORD } })).json()
  const auth = { Authorization: `Bearer ${login.access_token}` }
  const cpf = '222' + randomCPF().slice(3)
  const nine = cpf.slice(0, 9).split('').map(Number)
  for (const n of [9, 10]) {
    let sum = 0
    for (let i = 0; i < n; i++) sum += nine[i] * (n + 1 - i)
    nine.push(((sum * 10) % 11) % 10)
  }
  await request.put(api('/v1/me/profile'), { headers: auth, data: { full_name: name, cpf: nine.join(''), birth_date: '1985-03-10',
    address: { postal_code: '01310100', street: 'Av. Paulista', number: '10', city: 'São Paulo', state: 'SP' } } })
  const kyc = await (await request.post(api('/v1/me/kyc'), { headers: auth })).json()
  expect(kyc.kyc_status).toBe('MANUAL_REVIEW')

  await page.goto('/entrar')
  await page.getByLabel('E-mail').fill('analista@digitalbank.dev')
  await page.getByLabel('Senha').fill(PASSWORD)
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('heading', { name: 'Análises', exact: true })).toBeVisible()
  await page.getByRole('button', { name: new RegExp(name) }).click()
  await expect(page.getByText('Pessoa politicamente exposta').first()).toBeVisible()
  await shot(page, '10-backoffice')
  await page.getByRole('button', { name: 'Aprovar' }).click()
  await expect(page.getByText('Aprovado').first()).toBeVisible()

  const profile = await (await request.get(api('/v1/me/profile'), { headers: auth })).json()
  expect(profile.kyc_status).toBe('APPROVED')
})

test('customers cannot open the back office', async ({ page, request }) => {
  const email = `cli-${crypto.randomUUID().slice(0, 8)}@example.com`
  await request.post(api('/v1/auth/register'), { data: { email, password: PASSWORD, full_name: 'Bruno Lima' } })
  await page.goto('/entrar')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(PASSWORD)
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('heading', { name: 'Vamos abrir sua conta' })).toBeVisible()
  await page.goto('/backoffice')
  await expect(page.getByRole('heading', { name: 'Análises', exact: true })).toHaveCount(0)
})
