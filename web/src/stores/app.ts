import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type Theme = 'light' | 'dark'

interface AppState {
  theme: Theme
  currentCluster: string
  currentNamespace: string
  collapsed: boolean
  toggleTheme: () => void
  setCluster: (cluster: string) => void
  setNamespace: (ns: string) => void
  toggleCollapsed: () => void
}

export const useAppStore = create<AppState>()(
  persist(
    (set) => ({
      theme: 'dark',
      currentCluster: '',
      currentNamespace: '',
      collapsed: false,
      toggleTheme: () =>
        set((state) => ({ theme: state.theme === 'light' ? 'dark' : 'light' })),
      setCluster: (cluster) => set({ currentCluster: cluster }),
      setNamespace: (ns) => set({ currentNamespace: ns }),
      toggleCollapsed: () => set((state) => ({ collapsed: !state.collapsed })),
    }),
    {
      name: 'aiops-app-storage',
      partialize: (state) => ({
        theme: state.theme,
        currentCluster: state.currentCluster,
        currentNamespace: state.currentNamespace,
      }),
    },
  ),
)
