import { createContext, useContext } from 'react'

export const SessionContext = createContext(false)
export const LiveContext = createContext(false)

export const useSignedIn = () => useContext(SessionContext)
/** Whether the live notification stream is connected. */
export const useLiveConnected = () => useContext(LiveContext)
