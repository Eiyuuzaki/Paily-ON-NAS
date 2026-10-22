import { defineStore } from 'pinia'

const SERVICE_SECRET_KEY = 'paily_service_secret'
const THEME_KEY = 'paily_theme'

export const useSettingsStore = defineStore('settings', {
  state: (): { serviceSecret: string; theme: 'light' | 'dark' } => ({
    serviceSecret: localStorage.getItem(SERVICE_SECRET_KEY) || '',
    theme: (localStorage.getItem(THEME_KEY) as 'light' | 'dark') || 'light',
  }),

  actions: {
    setServiceSecret(secret: string): void {
      this.serviceSecret = secret
      localStorage.setItem(SERVICE_SECRET_KEY, secret)
    },
    setTheme(theme: 'light' | 'dark'): void {
      this.theme = theme
      localStorage.setItem(THEME_KEY, theme)
    },
    toggleTheme(): void {
      this.setTheme(this.theme === 'light' ? 'dark' : 'light')
    },
  },
})
