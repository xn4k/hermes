<script setup lang="ts">
import { onMounted, ref } from 'vue'
import FeedPanel from '../features/feed/FeedPanel.vue'
import NewsPanel from "../features/news/NewsPanel.vue";

type User = {
  id: number
  username: string
  displayName: string
}

type HealthResponse = {
  service: string
  status: string
  database: string
}

defineProps<{
  user: User
}>()

const emit = defineEmits<{
  (event: 'logout'): void
}>()

const health = ref<HealthResponse | null>(null)
const healthError = ref('')

onMounted(async () => {
  try {
    const response = await fetch('/api/health', {
      credentials: 'same-origin',
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    health.value = (await response.json()) as HealthResponse
  } catch (err) {
    healthError.value =
      err instanceof Error ? err.message : 'Status konnte nicht geladen werden'
  }
})
</script>

<template>
  <main class="dashboard">
    <section class="hero">
      <div>
        <p class="eyebrow">HERMES CAVE</p>
        <h1>Willkommen zurück, {{ user.displayName }}.</h1>
        <p class="subtitle">
          Private lokale Kommandozentrale. Feed zuerst, Chaos später.
        </p>
      </div>

      <button class="logout" type="button" @click="emit('logout')">
        Logout
      </button>
    </section>

    <section class="layout">
      <div class="main-content">
        <FeedPanel class="feed" />
        <NewsPanel />
      </div>

      <aside class="side">
        <article class="card">
          <p class="card-label">System</p>

          <div v-if="health" class="status-ok">
            <span class="indicator"></span>
            <div>
              <strong>Online</strong>
              <p>API: {{ health.status }}</p>
              <p>Datenbank: {{ health.database }}</p>
            </div>
          </div>

          <p v-else-if="healthError" class="error">
            {{ healthError }}
          </p>

          <p v-else>
            Lade Systemstatus …
          </p>
        </article>

        <article class="card">
          <p class="card-label">Identität</p>
          <p>User-ID: {{ user.id }}</p>
          <p>Username: {{ user.username }}</p>
        </article>

        <article class="card muted">
          <p class="card-label">Demnächst</p>
          <p>Quick Links, Wetter, Projekte, echte Hermes-Werte.</p>
        </article>
      </aside>
    </section>
  </main>
</template>

<style scoped>
.main-content {
  display: grid;
  gap: 1rem;
  min-width: 0;
}
.dashboard {
  min-height: 100vh;
  padding: 3rem;
}

.hero {
  display: flex;
  justify-content: space-between;
  gap: 2rem;
  align-items: flex-start;
  width: min(100%, 1180px);
  margin: 0 auto 2rem;
  padding: 2.5rem;
  border: 1px solid #273342;
  border-radius: 22px;
  background: rgba(15, 23, 32, 0.92);
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.35);
}

.eyebrow,
.card-label {
  margin: 0 0 0.7rem;
  color: #5ee6a8;
  font-family: monospace;
  letter-spacing: 0.18em;
}

h1 {
  margin: 0;
  max-width: 760px;
  font-size: clamp(2.2rem, 7vw, 4.2rem);
}

.subtitle {
  color: #a9b6c4;
  font-size: 1.1rem;
}

.logout {
  padding: 0.75rem 1rem;
  border: 1px solid #334155;
  border-radius: 10px;
  background: transparent;
  color: #edf4fa;
  cursor: pointer;
}

.logout:hover {
  border-color: #5ee6a8;
}

.layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 1rem;
  width: min(100%, 1180px);
  margin: 0 auto;
}

.feed {
  min-width: 0;
}

.side {
  display: grid;
  align-content: start;
  gap: 1rem;
}

.card {
  min-height: 150px;
  padding: 1.4rem;
  border: 1px solid #273342;
  border-radius: 18px;
  background: rgba(15, 23, 32, 0.86);
}

.card p {
  color: #a9b6c4;
}

.status-ok {
  display: flex;
  gap: 1rem;
  align-items: flex-start;
}

.indicator {
  width: 12px;
  height: 12px;
  margin-top: 0.3rem;
  border-radius: 50%;
  background: #5ee6a8;
  box-shadow: 0 0 18px #5ee6a8;
}

.error {
  color: #ff9baa;
}

.muted {
  opacity: 0.86;
}

@media (max-width: 900px) {
  .dashboard {
    padding: 1.2rem;
  }

  .hero {
    display: grid;
    padding: 1.5rem;
  }

  .layout {
    grid-template-columns: 1fr;
  }
}
</style>
