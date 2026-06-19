<script setup lang="ts">
import { ref } from 'vue'

type LoginPayload = {
  username: string
  password: string
}

defineProps<{
  error: string
  loading: boolean
}>()

const emit = defineEmits<{
  (event: 'login', payload: LoginPayload): void
}>()

const username = ref('mikhail')
const password = ref('')

function submitLogin() {
  emit('login', {
    username: username.value,
    password: password.value,
  })
}
</script>

<template>
  <main class="login-page">
    <section class="login-panel">
      <p class="eyebrow">PRIVATE SYSTEM</p>
      <h1>Hermes Cave</h1>
      <p class="subtitle">
        Authentifizierung erforderlich.
      </p>

      <form class="form" @submit.prevent="submitLogin">
        <label>
          Username
          <input
            v-model="username"
            name="username"
            autocomplete="username"
            required
          >
        </label>

        <label>
          Passwort
          <input
            v-model="password"
            name="password"
            type="password"
            autocomplete="current-password"
            required
          >
        </label>

        <p v-if="error" class="error">
          {{ error }}
        </p>

        <button type="submit" :disabled="loading">
          {{ loading ? 'Prüfe Zugang …' : 'Einloggen' }}
        </button>
      </form>
    </section>
  </main>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 2rem;
}

.login-panel {
  width: min(100%, 460px);
  padding: 2.5rem;
  border: 1px solid #273342;
  border-radius: 18px;
  background: rgba(15, 23, 32, 0.94);
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
  font-size: clamp(2.3rem, 8vw, 4.2rem);
}

.subtitle {
  color: #a9b6c4;
}

.form {
  display: grid;
  gap: 1rem;
  margin-top: 2rem;
}

label {
  display: grid;
  gap: 0.45rem;
  color: #c8d2dc;
}

input {
  width: 100%;
  padding: 0.85rem 1rem;
  border: 1px solid #334155;
  border-radius: 10px;
  background: #0b1119;
  color: #edf4fa;
  font: inherit;
}

input:focus {
  outline: 2px solid rgba(94, 230, 168, 0.55);
  border-color: #5ee6a8;
}

button {
  margin-top: 0.5rem;
  padding: 0.9rem 1rem;
  border: 0;
  border-radius: 10px;
  background: #5ee6a8;
  color: #07110c;
  font-weight: 700;
  cursor: pointer;
}

button:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}

.error {
  margin: 0;
  color: #ff9baa;
}
</style>
