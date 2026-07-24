<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

type NewsWarning = {
  sourceId: string
  sourceName: string
  message: string
}

type NewsCategory =
  | 'deutschland'
  | 'welt'
  | 'politik'
  | 'tech'
  | 'security'
  | 'wissenschaft'
  | 'kultur'
  | 'musik'
  | 'literatur'
  | 'wirtschaft'
  | 'sport'
  | 'wetter-klima'

type NewsArticle = {
  id: string
  sourceId: string
  sourceName: string
  category: NewsCategory
  title: string
  url: string
  summary: string
  publishedAt: string
}

type NewsResponse = {
  source: 'cache' | 'refresh' | 'manual_refresh'
  count: number
  articles: NewsArticle[]
  warnings: NewsWarning[]
}

const articles = ref<NewsArticle[]>([])
const source = ref<NewsResponse['source'] | null>(null)
const loading = ref(false)
const refreshing = ref(false)
const error = ref('')
const activeCategory = ref<NewsCategory | 'alle'>('alle')
const warnings = ref<NewsWarning[]>([])

const categories: Array<{ key: NewsCategory | 'alle'; label: string }> = [
  { key: 'alle', label: 'Alle' },
  { key: 'deutschland', label: 'Deutschland' },
  { key: 'welt', label: 'Welt' },
  { key: 'politik', label: 'Politik' },
  { key: 'tech', label: 'Tech' },
  { key: 'security', label: 'Security' },
  { key: 'wissenschaft', label: 'Wissenschaft' },
  { key: 'kultur', label: 'Kultur' },
  { key: 'musik', label: 'Musik' },
  { key: 'literatur', label: 'Literatur' },
  { key: 'wirtschaft', label: 'Wirtschaft' },
  { key: 'sport', label: 'Sport' },
  { key: 'wetter-klima', label: 'Wetter & Klima' },
]

const visibleArticles = computed(() => {
  if (activeCategory.value === 'alle') {
    return articles.value
  }

  return articles.value.filter((article) => article.category === activeCategory.value)
})

const sourceLabel = computed(() => {
  if (source.value === 'cache') {
    return 'aus Cache'
  }

  if (source.value === 'refresh') {
    return 'gerade geladen'
  }

  if (source.value === 'manual_refresh') {
    return 'manuell aktualisiert'
  }

  return ''
})

async function requestNews(url: string, method = 'GET') {
  const response = await fetch(url, {
    method,
    credentials: 'include',
  })

  const data = (await response.json()) as NewsResponse & {
    error?: string
    details?: string
  }

  if (!response.ok) {
    throw new Error(data.details || data.error || 'News konnten nicht geladen werden')
  }

  return data
}

async function loadNews() {
  loading.value = true
  error.value = ''

  try {
    const data = await requestNews('/api/news')

    articles.value = data.articles
    source.value = data.source
    warnings.value = data.warnings ?? []
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'News konnten nicht geladen werden'
  } finally {
    loading.value = false
  }
}

async function refreshNews() {
  refreshing.value = true
  error.value = ''

  try {
    const data = await requestNews('/api/news/refresh', 'POST')

    articles.value = data.articles
    source.value = data.source
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'News konnten nicht aktualisiert werden'
  } finally {
    refreshing.value = false
  }
}

function formatDate(value: string) {
  if (!value) {
    return ''
  }

  const date = new Date(value)

  if (Number.isNaN(date.getTime())) {
    return ''
  }

  return new Intl.DateTimeFormat('de-DE', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date)
}

function cleanSummary(value: string) {
  return value
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

onMounted(loadNews)
</script>

<template>
  <section class="news-panel">
    <header class="news-header">
      <div>
        <p class="eyebrow">EXTERNAL SIGNALS</p>
        <h2>News Radar</h2>
        <p v-if="sourceLabel" class="status-text">
          {{ articles.length }} Artikel · {{ sourceLabel }}
        </p>
      </div>

      <button
        class="refresh-button"
        type="button"
        :disabled="loading || refreshing"
        @click="refreshNews"
      >
        {{ refreshing ? 'Aktualisiert...' : 'Refresh' }}
      </button>
    </header>

    <div class="category-list">
      <button
        v-for="category in categories"
        :key="category.key"
        class="category-button"
        :class="{ active: activeCategory === category.key }"
        type="button"
        @click="activeCategory = category.key"
      >
        {{ category.label }}
      </button>
    </div>

    <div v-if="warnings.length > 0" class="warning-box">
      <strong>Einige Quellen konnten nicht geladen werden.</strong>

      <ul>
        <li v-for="warning in warnings" :key="warning.sourceId">
          {{ warning.sourceName }}: {{ warning.message }}
        </li>
      </ul>
    </div>

    <p v-if="loading" class="info-text">Hermes sammelt Nachrichten ein.</p>

    <p v-else-if="error" class="error-text">
      {{ error }}
    </p>

    <p v-else-if="visibleArticles.length === 0" class="info-text">
      Für diese Kategorie gibt es gerade keine Artikel.
    </p>

    <div v-else class="article-list">
      <article v-for="article in visibleArticles" :key="article.id" class="article-card">
        <div class="article-meta">
          <span class="category-label">{{ article.category }}</span>
          <span>{{ article.sourceName }}</span>
        </div>

        <a
          v-if="article.url"
          class="article-title"
          :href="article.url"
          target="_blank"
          rel="noopener noreferrer"
        >
          {{ article.title }}
        </a>

        <h3 v-else class="article-title">
          {{ article.title }}
        </h3>

        <p v-if="cleanSummary(article.summary)" class="article-summary">
          {{ cleanSummary(article.summary) }}
        </p>

        <time v-if="formatDate(article.publishedAt)" class="article-date">
          {{ formatDate(article.publishedAt) }}
        </time>
      </article>
    </div>
  </section>
</template>

<style scoped>
.news-panel {
  display: grid;
  gap: 1rem;
}

.news-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.eyebrow {
  margin: 0 0 0.25rem;
  font-size: 0.72rem;
  letter-spacing: 0.14em;
  opacity: 0.65;
}

h2 {
  margin: 0;
}

.status-text,
.info-text,
.article-date {
  margin: 0.35rem 0 0;
  font-size: 0.85rem;
  opacity: 0.7;
}

.refresh-button,
.category-button {
  border: 1px solid currentColor;
  border-radius: 0.5rem;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.refresh-button {
  padding: 0.55rem 0.8rem;
}

.refresh-button:disabled {
  cursor: wait;
  opacity: 0.55;
}

.category-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.category-button {
  padding: 0.35rem 0.6rem;
  font-size: 0.8rem;
  opacity: 0.7;
}

.category-button.active {
  opacity: 1;
  font-weight: 700;
}

.article-list {
  display: grid;
  gap: 0.8rem;
}

.article-card {
  display: grid;
  gap: 0.5rem;
  padding: 1rem;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 0.75rem;
}

.article-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  font-size: 0.78rem;
  opacity: 0.7;
}

.category-label {
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.article-title {
  color: inherit;
  font-size: 1rem;
  font-weight: 700;
  text-decoration: none;
}

a.article-title:hover {
  text-decoration: underline;
}

.article-summary {
  margin: 0;
  line-height: 1.5;
  opacity: 0.82;
}

.error-text {
  margin: 0;
  color: #e07a7a;
}
.warning-box {
  padding: 0.9rem 1rem;
  border: 1px solid rgba(255, 190, 100, 0.45);
  border-radius: 0.75rem;
  background: rgba(255, 190, 100, 0.08);
  font-size: 0.86rem;
}

.warning-box ul {
  margin: 0.5rem 0 0;
  padding-left: 1.1rem;
}

.warning-box li {
  margin: 0.25rem 0;
  opacity: 0.8;
}
</style>
