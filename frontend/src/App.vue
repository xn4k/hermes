<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { apiRequest, ApiError, setUnauthorizedHandler } from './api'
import LoginView from './views/LoginView.vue'
import DashboardView from './views/DashboardView.vue'

type User = {
  id: number
  username: string
  displayName: string
}

type MeResponse = {
  user: User
}

type LoginPayload = {
  username: string
  password: string
}

const user = ref<User | null>(null)
const booting = ref(true)
const bootError = ref('')
const authError = ref('')
const authLoading = ref(false)

function handleUnauthorized() {
  user.value = null
  authError.value = 'Deine Sitzung ist abgelaufen. Bitte melde dich erneut an.'
}

async function loadMe() {
  booting.value = true
  bootError.value = ''

  try {
    const data = await apiRequest<MeResponse>('/api/me', {
      notifyUnauthorized: false,
    })
    user.value = data.user
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      user.value = null
      return
    }

    bootError.value = err instanceof Error ? err.message : 'Unbekannter Fehler'
  } finally {
    booting.value = false
  }
}

async function handleLogin(payload: LoginPayload) {
  authLoading.value = true
  authError.value = ''

  try {
    const data = await apiRequest<MeResponse>('/api/login', {
      method: 'POST',
      notifyUnauthorized: false,
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    })

    user.value = data.user
  } catch (err) {
    authError.value =
      err instanceof ApiError && err.code === 'invalid_credentials'
        ? 'Username oder Passwort ist falsch.'
        : err instanceof Error
          ? err.message
          : 'Unbekannter Fehler'
  } finally {
    authLoading.value = false
  }
}

async function handleLogout() {
  authError.value = ''

  try {
    await apiRequest<{ status: string }>('/api/logout', {
      method: 'POST',
      notifyUnauthorized: false,
    })
  } catch (err) {
    authError.value =
      err instanceof Error
        ? `Logout konnte serverseitig nicht bestätigt werden: ${err.message}`
        : 'Logout konnte serverseitig nicht bestätigt werden.'
  }

  user.value = null
}

onMounted(() => {
  setUnauthorizedHandler(handleUnauthorized)
  void loadMe()
})

onBeforeUnmount(() => {
  setUnauthorizedHandler(null)
})
</script>

<template>
  <main v-if="booting" class="shell">
    <section class="panel">
      <p class="eyebrow">HERMES</p>
      <h1>System startet …</h1>
    </section>
  </main>

  <main v-else-if="bootError" class="shell">
    <section class="panel error-panel">
      <p class="eyebrow">SYSTEM ERROR</p>
      <h1>Hermes antwortet nicht sauber.</h1>
      <p>{{ bootError }}</p>
    </section>
  </main>

  <LoginView
    v-else-if="!user"
    :error="authError"
    :loading="authLoading"
    @login="handleLogin"
  />

  <DashboardView
    v-else
    :user="user"
    @logout="handleLogout"
  />
</template>

<style scoped>
.shell {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 2rem;
}

.panel {
  width: min(100%, 620px);
  padding: 2.5rem;
  border: 1px solid #273342;
  border-radius: 18px;
  background: rgba(15, 23, 32, 0.92);
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.35);
}

.eyebrow {
  margin: 0 0 0.7rem;
  color: #5ee6a8;
  font-family: monospace;
  letter-spacing: 0.18em;
}

h1 {
  margin: 0;
}

.error-panel {
  border-color: #b94c5d;
}
</style>
