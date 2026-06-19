<script setup lang="ts">
import { onMounted, ref } from 'vue'
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

async function loadMe() {
  booting.value = true
  bootError.value = ''

  try {
    const response = await fetch('/api/me', {
      credentials: 'same-origin',
    })

    if (response.status === 401) {
      user.value = null
      return
    }

    if (!response.ok) {
      throw new Error(`API antwortet mit HTTP ${response.status}`)
    }

    const data = (await response.json()) as MeResponse
    user.value = data.user
  } catch (err) {
    bootError.value =
      err instanceof Error ? err.message : 'Unbekannter Fehler'
  } finally {
    booting.value = false
  }
}

async function handleLogin(payload: LoginPayload) {
  authLoading.value = true
  authError.value = ''

  try {
    const response = await fetch('/api/login', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    })

    if (!response.ok) {
      let message = 'Login fehlgeschlagen'

      try {
        const data = (await response.json()) as { error?: string }

        if (data.error === 'invalid_credentials') {
          message = 'Username oder Passwort ist falsch.'
        }
      } catch {
        // Antwort war kein JSON. Drama, aber kontrolliertes Drama.
      }

      throw new Error(message)
    }

    const data = (await response.json()) as MeResponse
    user.value = data.user
  } catch (err) {
    authError.value =
      err instanceof Error ? err.message : 'Unbekannter Fehler'
  } finally {
    authLoading.value = false
  }
}

async function handleLogout() {
  await fetch('/api/logout', {
    method: 'POST',
    credentials: 'same-origin',
  })

  user.value = null
}

onMounted(loadMe)
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
