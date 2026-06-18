<script setup lang="ts">
import { onMounted, ref } from 'vue'

type HealthResponse = {
  service: string
  status: string
  database: string
}

const health = ref<HealthResponse | null>(null)
const error = ref('')
const loading = ref(true)

onMounted(async () => {
  try {
    const response = await fetch('/api/health')

    if (!response.ok) {
      throw new Error(`API antwortet mit HTTP ${response.status}`)
    }

    health.value = await response.json()
  } catch (err) {
    error.value =
      err instanceof Error ? err.message : 'Unbekannter Fehler'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <main class="dashboard">
    <section class="panel">
      <p class="eyebrow">PRIVATE SYSTEM</p>
      <h1>Hermes Cave</h1>
      <p class="welcome">Willkommen zurück, Mikhail.</p>

      <div v-if="loading" class="status">
        Systemstatus wird geladen …
      </div>

      <div v-else-if="error" class="status status-error">
        {{ error }}
      </div>

      <div v-else-if="health" class="status status-ok">
        <span class="indicator"></span>

        <div>
          <strong>System online</strong>
          <p>API: {{ health.status }}</p>
          <p>Datenbank: {{ health.database }}</p>
        </div>
      </div>
    </section>
  </main>
</template>

<style scoped>
.dashboard {
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
  font-size: clamp(2.3rem, 8vw, 4.5rem);
}

.welcome {
  margin: 1rem 0 2rem;
  color: #a9b6c4;
  font-size: 1.1rem;
}

.status {
  display: flex;
  gap: 1rem;
  align-items: flex-start;
  padding: 1.2rem;
  border: 1px solid #334155;
  border-radius: 12px;
  background: #111a24;
}

.status p {
  margin: 0.3rem 0 0;
  color: #9eacba;
}

.status-ok {
  border-color: rgba(94, 230, 168, 0.45);
}

.status-error {
  border-color: #b94c5d;
  color: #ff9baa;
}

.indicator {
  width: 12px;
  height: 12px;
  margin-top: 0.3rem;
  border-radius: 50%;
  background: #5ee6a8;
  box-shadow: 0 0 18px #5ee6a8;
}
</style>
