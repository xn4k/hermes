<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { apiRequest } from '../../api'

type FeedEntry = {
  id: number
  type: string
  title: string
  content: string
  pinned: boolean
  createdAt: string
  updatedAt: string
}

type FeedResponse = {
  entries: FeedEntry[]
}

type CreateFeedResponse = {
  entry: FeedEntry
}

const entries = ref<FeedEntry[]>([])
const title = ref('')
const content = ref('')
const loading = ref(true)
const saving = ref(false)
const actionEntryId = ref<number | null>(null)
const error = ref('')

function formatDate(value: string) {
  return new Intl.DateTimeFormat('de-DE', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

async function loadFeed() {
  loading.value = true
  error.value = ''

  try {
    const data = await apiRequest<FeedResponse>('/api/feed')
    entries.value = data.entries
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Unbekannter Fehler'
  } finally {
    loading.value = false
  }
}

async function createEntry() {
  const trimmedContent = content.value.trim()

  if (!trimmedContent) {
    return
  }

  saving.value = true
  error.value = ''

  try {
    const data = await apiRequest<CreateFeedResponse>('/api/feed', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        title: title.value.trim(),
        content: trimmedContent,
        type: 'note',
      }),
    })

    entries.value = [data.entry, ...entries.value]
    title.value = ''
    content.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Unbekannter Fehler'
  } finally {
    saving.value = false
  }
}

async function deleteEntry(entryId: number) {
  actionEntryId.value = entryId
  error.value = ''

  try {
    await apiRequest<{ status: string }>(`/api/feed/${entryId}`, {
      method: 'DELETE',
    })
    entries.value = entries.value.filter((entry) => entry.id !== entryId)
  } catch (err) {
    error.value =
      err instanceof Error ? err.message : 'Eintrag konnte nicht gelöscht werden'
  } finally {
    actionEntryId.value = null
  }
}

async function togglePin(entryId: number) {
  actionEntryId.value = entryId
  error.value = ''

  try {
    const data = await apiRequest<CreateFeedResponse>(`/api/feed/${entryId}/pin`, {
      method: 'PATCH',
    })

    entries.value = entries.value
      .map((entry) => (entry.id === entryId ? data.entry : entry))
      .sort(
        (a, b) =>
          Number(b.pinned) - Number(a.pinned) ||
          new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
      )
  } catch (err) {
    error.value =
      err instanceof Error ? err.message : 'Pin-Status konnte nicht geändert werden'
  } finally {
    actionEntryId.value = null
  }
}

onMounted(loadFeed)
</script>

<template>
  <section class="feed-panel">
    <header class="feed-header">
      <div>
        <p class="eyebrow">PRIVATE FEED</p>
        <h2>Gedankenstrom</h2>
      </div>

      <button type="button" class="ghost" :disabled="loading" @click="loadFeed">
        Refresh
      </button>
    </header>

    <form class="composer" @submit.prevent="createEntry">
      <input
        v-model="title"
        placeholder="Titel optional"
        aria-label="Titel"
      >

      <textarea
        v-model="content"
        placeholder="Was geht dir durch den Kopf?"
        aria-label="Inhalt"
        rows="4"
        required
      />

      <div class="composer-actions">
        <p v-if="error" class="error">
          {{ error }}
        </p>

        <button type="submit" :disabled="saving || !content.trim()">
          {{ saving ? 'Speichere …' : 'Posten' }}
        </button>
      </div>
    </form>

    <div v-if="loading" class="empty">
      Feed wird geladen …
    </div>

    <div v-else-if="entries.length === 0" class="empty">
      Noch keine Einträge. Hermes wartet auf deine ersten Gedanken.
      Bedrohlich geduldig.
    </div>

    <article
      v-for="entry in entries"
      v-else
      :key="entry.id"
      class="entry"
      :class="{ pinned: entry.pinned }"
    >
      <div class="entry-top">
        <div>
          <p class="meta">
            {{ entry.type }} · {{ formatDate(entry.createdAt) }}
            <span v-if="entry.pinned"> · pinned</span>
          </p>

          <h3 v-if="entry.title">
            {{ entry.title }}
          </h3>
        </div>

        <div class="entry-actions">
          <button
            type="button"
            :disabled="actionEntryId === entry.id"
            @click="togglePin(entry.id)"
          >
            {{ entry.pinned ? 'Unpin' : 'Pin' }}
          </button>

          <button
            type="button"
            class="danger"
            :disabled="actionEntryId === entry.id"
            @click="deleteEntry(entry.id)"
          >
            Löschen
          </button>
        </div>
      </div>

      <p class="content">
        {{ entry.content }}
      </p>
    </article>
  </section>
</template>

<style scoped>
.feed-panel {
  display: grid;
  gap: 1rem;
}

.feed-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.eyebrow {
  margin: 0 0 0.45rem;
  color: #5ee6a8;
  font-family: monospace;
  letter-spacing: 0.18em;
}

h2 {
  margin: 0;
  font-size: 2rem;
}

.composer,
.entry,
.empty {
  padding: 1.2rem;
  border: 1px solid #273342;
  border-radius: 18px;
  background: rgba(15, 23, 32, 0.86);
}

.composer {
  display: grid;
  gap: 0.8rem;
}

input,
textarea {
  width: 100%;
  padding: 0.85rem 1rem;
  border: 1px solid #334155;
  border-radius: 12px;
  background: #0b1119;
  color: #edf4fa;
  font: inherit;
}

textarea {
  resize: vertical;
}

input:focus,
textarea:focus {
  outline: 2px solid rgba(94, 230, 168, 0.55);
  border-color: #5ee6a8;
}

.composer-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

button {
  padding: 0.65rem 0.9rem;
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

.ghost {
  border: 1px solid #334155;
  background: transparent;
  color: #edf4fa;
}

.entry {
  display: grid;
  gap: 0.8rem;
}

.entry.pinned {
  border-color: rgba(94, 230, 168, 0.65);
}

.entry-top {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
}

.entry-actions {
  display: flex;
  gap: 0.5rem;
}

.entry-actions button {
  border: 1px solid #334155;
  background: transparent;
  color: #edf4fa;
}

.entry-actions .danger {
  border-color: rgba(185, 76, 93, 0.65);
  color: #ff9baa;
}

.meta {
  margin: 0;
  color: #7f8ea3;
  font-size: 0.85rem;
}

h3 {
  margin: 0.35rem 0 0;
}

.content {
  margin: 0;
  white-space: pre-wrap;
  color: #c8d2dc;
  line-height: 1.6;
}

.empty {
  color: #a9b6c4;
}

.error {
  margin: 0;
  color: #ff9baa;
}
</style>
