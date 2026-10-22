import { defineStore } from 'pinia'
import { login as apiLogin, refreshToken as apiRefresh } from '../api/auth'

const ACCESS_TOKEN_KEY = 'paily_access_token'
const REFRESH_TOKEN_KEY = 'paily_refresh_token'

export const useAuthStore = defineStore('auth', {
  state: (): { accessToken: string; refreshToken: string } => ({
    accessToken: '',
    refreshToken: '',
  }),

  getters: {
    isLoggedIn: (state): boolean => !!state.accessToken,
  },

  actions: {
    async login(password: string): Promise<void> {
      const res = await apiLogin(password)
      this.accessToken = res.data.token
      this.refreshToken = res.data.refresh_token
      localStorage.setItem(ACCESS_TOKEN_KEY, this.accessToken)
      localStorage.setItem(REFRESH_TOKEN_KEY, this.refreshToken)
    },

    async refreshAccessToken(): Promise<void> {
      if (!this.refreshToken) throw new Error('No refresh token')
      const res = await apiRefresh(this.refreshToken)
      this.accessToken = res.data.token
      localStorage.setItem(ACCESS_TOKEN_KEY, this.accessToken)
    },

    logout(): void {
      this.accessToken = ''
      this.refreshToken = ''
      localStorage.removeItem(ACCESS_TOKEN_KEY)
      localStorage.removeItem(REFRESH_TOKEN_KEY)
    },

    restoreSession(): void {
      this.accessToken = localStorage.getItem(ACCESS_TOKEN_KEY) || ''
      this.refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY) || ''
    },
  },
})
