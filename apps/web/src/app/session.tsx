import { useEffect, useState, type ReactNode } from 'react'
import { hasSession, onSessionChange } from '../lib/api'
import { SessionContext } from './contexts'

export function SessionProvider({ children }: { children: ReactNode }) {
  const [signedIn, setSignedIn] = useState(hasSession)
  useEffect(() => {
    const off = onSessionChange(setSignedIn)
    return () => {
      off()
    }
  }, [])
  return <SessionContext.Provider value={signedIn}>{children}</SessionContext.Provider>
}
