import { lazy, Suspense } from 'react'
import { BrowserRouter, Navigate, Outlet, Route, Routes } from 'react-router'
import { LiveNotifications } from './app/live'
import { useAccounts, useMe, useProfile } from './app/queries'
import { useSignedIn } from './app/contexts'
import { Layout, MorePage } from './components/Layout'
import { Loading } from './components/ui'
import { LoginPage, RegisterPage } from './pages/Auth'
import { HomePage, StatementPage } from './pages/Home'
import { OnboardingPage } from './pages/Onboarding'

// Less visited screens load on demand (smaller first download).
const page = <K extends string>(load: () => Promise<Record<K, React.ComponentType>>, name: K) =>
  lazy(() => load().then((m) => ({ default: m[name] })))
const PixPage = page(() => import('./pages/Pix'), 'PixPage')
const CardsPage = page(() => import('./pages/Cards'), 'CardsPage')
const CreditPage = page(() => import('./pages/Credit'), 'CreditPage')
const BoletosPage = page(() => import('./pages/Boletos'), 'BoletosPage')
const SavingsPage = page(() => import('./pages/Savings'), 'SavingsPage')
const SpendingPage = page(() => import('./pages/Spending'), 'SpendingPage')
const VerifyReceiptPage = page(() => import('./pages/VerifyReceipt'), 'VerifyReceiptPage')
const NotificationsPage = page(() => import('./pages/Notifications'), 'NotificationsPage')
const SettingsPage = page(() => import('./pages/Settings'), 'SettingsPage')
const BackofficePage = page(() => import('./pages/Backoffice'), 'BackofficePage')

function SignedOut() {
  return useSignedIn() ? <Navigate to="/" replace /> : <Outlet />
}

function SignedIn() {
  if (!useSignedIn()) return <Navigate to="/entrar" replace />
  return <LiveNotifications><Outlet /></LiveNotifications>
}

/** The app needs a verified identity and an account; otherwise onboarding. */
function useReady() {
  const me = useMe()
  const profile = useProfile()
  const accounts = useAccounts()
  if (me.isLoading || profile.isLoading || accounts.isLoading) return undefined
  if (me.data?.role === 'ANALYST') return 'staff' as const
  return profile.data?.kyc_status === 'APPROVED' && (accounts.data?.length ?? 0) > 0
}

function Ready() {
  const ready = useReady()
  if (ready === undefined) return <Loading />
  if (ready === 'staff') return <Navigate to="/backoffice" replace />
  return ready ? <Outlet /> : <Navigate to="/cadastro" replace />
}

/** Bank staff only; customers go back to their app. */
function Staff() {
  const me = useMe()
  if (me.isLoading) return <Loading />
  return me.data?.role === 'ANALYST' ? <Suspense fallback={<Loading />}><BackofficePage /></Suspense> : <Navigate to="/" replace />
}

function Onboarding() {
  const ready = useReady()
  if (ready === undefined) return <Loading />
  if (ready === 'staff') return <Navigate to="/backoffice" replace />
  return ready ? <Navigate to="/" replace /> : <OnboardingPage />
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<SignedOut />}>
          <Route path="/entrar" element={<LoginPage />} />
          <Route path="/criar-conta" element={<RegisterPage />} />
        </Route>
        <Route element={<SignedIn />}>
          <Route path="/cadastro" element={<Onboarding />} />
          <Route path="/backoffice" element={<Staff />} />
          <Route element={<Ready />}>
            <Route element={<Suspense fallback={<Loading />}><Layout /></Suspense>}>
              <Route index element={<HomePage />} />
              <Route path="/extrato" element={<StatementPage />} />
              <Route path="/pix" element={<PixPage />} />
              <Route path="/cartoes" element={<CardsPage />} />
              <Route path="/credito" element={<CreditPage />} />
              <Route path="/boletos" element={<BoletosPage />} />
              <Route path="/caixinhas" element={<SavingsPage />} />
              <Route path="/gastos" element={<SpendingPage />} />
              <Route path="/notificacoes" element={<NotificationsPage />} />
              <Route path="/ajustes" element={<SettingsPage />} />
              <Route path="/mais" element={<MorePage />} />
            </Route>
          </Route>
        </Route>
        <Route path="/comprovante/:code" element={<Suspense fallback={<Loading />}><VerifyReceiptPage /></Suspense>} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
