import { BrowserRouter, Navigate, Outlet, Route, Routes } from 'react-router'
import { LiveNotifications } from './app/live'
import { useAccounts, useProfile } from './app/queries'
import { useSignedIn } from './app/contexts'
import { Layout, MorePage } from './components/Layout'
import { Loading } from './components/ui'
import { LoginPage, RegisterPage } from './pages/Auth'
import { BoletosPage } from './pages/Boletos'
import { CardsPage } from './pages/Cards'
import { CreditPage } from './pages/Credit'
import { HomePage, StatementPage } from './pages/Home'
import { NotificationsPage } from './pages/Notifications'
import { OnboardingPage } from './pages/Onboarding'
import { PixPage } from './pages/Pix'
import { SettingsPage } from './pages/Settings'

function SignedOut() {
  return useSignedIn() ? <Navigate to="/" replace /> : <Outlet />
}

function SignedIn() {
  if (!useSignedIn()) return <Navigate to="/entrar" replace />
  return <LiveNotifications><Outlet /></LiveNotifications>
}

/** The app needs a verified identity and an account; otherwise onboarding. */
function useReady() {
  const profile = useProfile()
  const accounts = useAccounts()
  if (profile.isLoading || accounts.isLoading) return undefined
  return profile.data?.kyc_status === 'APPROVED' && (accounts.data?.length ?? 0) > 0
}

function Ready() {
  const ready = useReady()
  if (ready === undefined) return <Loading />
  return ready ? <Outlet /> : <Navigate to="/cadastro" replace />
}

function Onboarding() {
  const ready = useReady()
  if (ready === undefined) return <Loading />
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
          <Route element={<Ready />}>
            <Route element={<Layout />}>
              <Route index element={<HomePage />} />
              <Route path="/extrato" element={<StatementPage />} />
              <Route path="/pix" element={<PixPage />} />
              <Route path="/cartoes" element={<CardsPage />} />
              <Route path="/credito" element={<CreditPage />} />
              <Route path="/boletos" element={<BoletosPage />} />
              <Route path="/notificacoes" element={<NotificationsPage />} />
              <Route path="/ajustes" element={<SettingsPage />} />
              <Route path="/mais" element={<MorePage />} />
            </Route>
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
