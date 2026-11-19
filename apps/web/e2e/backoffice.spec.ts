import { expect, test } from '@playwright/test'
import { ANALYST, fund, onboard, PASSWORD, randomCPF, REVIEWER, shot, staffLogin } from './helpers'

const api = (path: string) => `http://127.0.0.1:8000${path}`

// Analyst logins share TOTP codes (one use each): run these one at a time.
test.describe.configure({ mode: 'serial', timeout: 150_000 })

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

  await staffLogin(page, ANALYST)
  await page.getByRole('button', { name: new RegExp(name) }).click()
  await expect(page.getByText('Pessoa politicamente exposta').first()).toBeVisible()
  await shot(page, '10-backoffice')
  await page.getByRole('button', { name: 'Aprovar' }).click()
  await expect(page.getByText('Aprovado').first()).toBeVisible()

  const profile = await (await request.get(api('/v1/me/profile'), { headers: auth })).json()
  expect(profile.kyc_status).toBe('APPROVED')
})

test('unblocking an account needs a second analyst', async ({ page, browser, request }) => {
  // A verified customer with an account.
  const email = `bo-${crypto.randomUUID().slice(0, 8)}@example.com`
  const name = 'Davi Rocha'
  await request.post(api('/v1/auth/register'), { data: { email, password: PASSWORD, full_name: name } })
  const login = await (await request.post(api('/v1/auth/login'), { data: { email, password: PASSWORD } })).json()
  const auth = { Authorization: `Bearer ${login.access_token}` }
  await request.put(api('/v1/me/profile'), { headers: auth, data: { full_name: name, cpf: randomCPF(), birth_date: '1990-01-15',
    address: { postal_code: '01310100', street: 'Av. Paulista', number: '20', city: 'São Paulo', state: 'SP' } } })
  expect((await (await request.post(api('/v1/me/kyc'), { headers: auth })).json()).kyc_status).toBe('APPROVED')
  const account = await (await request.post(api('/v1/accounts'), { headers: { ...auth, 'Idempotency-Key': crypto.randomUUID() }, data: {} })).json()
  const me = await (await request.get(api('/v1/me'), { headers: auth })).json()

  // First analyst: blocks the account, then asks to unblock it.
  await staffLogin(page, ANALYST)
  await page.getByLabel('Buscar cliente pelo id').fill(me.customer_id)
  await page.getByRole('button', { name: 'Abrir' }).click()
  await page.getByLabel('Motivo do bloqueio').fill('Suspeita de fraude')
  await page.getByRole('button', { name: 'Bloquear conta' }).click()
  await page.getByLabel('Motivo do desbloqueio').fill('Cliente confirmou as compras')
  await page.getByRole('button', { name: 'Pedir desbloqueio' }).click()
  await expect(page.getByText('Pedido enviado')).toBeVisible()
  await expect(page.getByText('Aguardando outro analista').first()).toBeVisible()
  const status = async () => (await (await request.get(api(`/v1/accounts/${account.id}`), { headers: auth })).json()).status
  expect(await status()).toBe('BLOCKED')

  // Second analyst, in another browser: approves, and the account is active again.
  const other = await browser.newContext()
  const reviewer = await other.newPage()
  await staffLogin(reviewer, REVIEWER)
  const item = reviewer.getByRole('listitem').filter({ hasText: `${name} · conta ${account.number}` })
  await item.getByRole('button', { name: 'Aprovar' }).click()
  await expect.poll(status).toBe('ACTIVE')
  await reviewer.getByLabel('Buscar cliente pelo id').fill(me.customer_id)
  await reviewer.getByRole('button', { name: 'Abrir' }).click()
  await expect(reviewer.getByText('Conta desbloqueada')).toBeVisible()
  await expect(reviewer.getByText(/Cadeia íntegra/)).toBeVisible()
  await shot(reviewer, '11-backoffice-four-eyes')
  await other.close()
})

test('a customer disputes a card purchase and an analyst refunds it', async ({ page, browser }) => {
  await onboard(page)
  await fund(page, '200,00')
  await page.goto('/cartoes')
  await page.getByRole('button', { name: 'Criar cartão virtual' }).click()
  const merchant = `LOJA ${crypto.randomUUID().slice(0, 6).toUpperCase()}`
  await page.getByLabel('Simular compra de').fill('75')
  await page.getByLabel('Loja', { exact: true }).fill(merchant)
  await page.getByRole('button', { name: 'Passar o cartão' }).click()
  await expect(page.getByText('Confirme que é você')).toBeVisible()
  await page.getByLabel('Senha').fill(PASSWORD)
  await page.getByRole('button', { name: 'Confirmar' }).click()
  await expect(page.getByText(/Aprovada: R\$ 75,00/)).toBeVisible()

  const row = page.getByRole('listitem').filter({ hasText: merchant })
  await row.getByRole('button', { name: 'Contestar' }).click()
  await page.getByLabel('O que aconteceu?').selectOption('NOT_RECEIVED')
  await page.getByLabel('Conte o que houve (opcional)').fill('O pedido nunca chegou')
  await page.getByRole('button', { name: 'Enviar contestação' }).click()
  await expect(row.getByText('Contestação em análise')).toBeVisible()
  await shot(page, '12-dispute')

  const other = await browser.newContext()
  const staff = await other.newPage()
  await staffLogin(staff, REVIEWER)
  const item = staff.getByRole('listitem').filter({ hasText: merchant })
  await expect(item.getByText('O pedido nunca chegou')).toBeVisible()
  await item.getByRole('button', { name: 'Aceitar e estornar' }).click()
  await expect(item).toHaveCount(0)
  await other.close()

  await page.reload()
  await expect(page.getByRole('listitem').filter({ hasText: merchant }).getByText('Contestação estornada')).toBeVisible()
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
