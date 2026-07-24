<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiRequest } from '../../api'

type WeatherLocation = {
  name: string
  admin1: string
  country: string
  latitude: number
  longitude: number
  timezone: string
}

type WeatherCurrent = {
  time: string
  temperature: number
  apparentTemperature: number
  weatherCode: number
  windSpeed: number
}

type WeatherHourlyPoint = {
  time: string
  temperature: number
  precipitationProbability: number
  precipitation: number
  weatherCode: number
}

type WeatherDailyPoint = {
  date: string
  temperatureMin: number
  temperatureMax: number
  apparentTemperatureMax: number
  precipitationProbability: number
  weatherCode: number
  sunrise: string
  sunset: string
}

type WeatherModel = {
  id: string
  name: string
  short: string
  color: string
  hourly: WeatherHourlyPoint[]
  daily: WeatherDailyPoint[]
}

type WeatherConsensus = {
  todayMaxMedian: number
  todayMaxMin: number
  todayMaxMax: number
  temperatureSpread: number
  confidence: 'high' | 'medium' | 'low'
  rainAgreementNext6: number
  rainProbabilityNext6: number
  rainStart: string
  heatLevel: 'normal' | 'notice' | 'warning' | 'danger'
  heatMessage: string
}

type WeatherForecast = {
  location: WeatherLocation
  current: WeatherCurrent
  models: WeatherModel[]
  consensus: WeatherConsensus
  refreshedAt: string
  source: 'refresh' | 'cache' | 'stale'
}

type LocationSearchResponse = {
  locations: WeatherLocation[]
}

const forecast = ref<WeatherForecast | null>(null)
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const showSettings = ref(false)
const locationQuery = ref('51069')
const locationResults = ref<WeatherLocation[]>([])
const locationError = ref('')
const searching = ref(false)
const savingLocation = ref(false)

function median(values: number[]) {
  if (values.length === 0) {
    return 0
  }

  const sorted = [...values].sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)

  return sorted.length % 2 === 0 ? (sorted[middle - 1]! + sorted[middle]!) / 2 : sorted[middle]!
}

function formatTemperature(value: number) {
  return `${Math.round(value)}°`
}

function formatHour(value: string) {
  if (!value) {
    return ''
  }

  return new Intl.DateTimeFormat('de-DE', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function formatDay(value: string) {
  return new Intl.DateTimeFormat('de-DE', {
    weekday: 'short',
    day: '2-digit',
    month: '2-digit',
  }).format(new Date(`${value}T12:00:00`))
}

function formatRefreshedAt(value: string) {
  return new Intl.DateTimeFormat('de-DE', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function weatherLabel(code: number) {
  if (code === 0) return 'Klar'
  if ([1, 2].includes(code)) return 'Leicht bewölkt'
  if (code === 3) return 'Bedeckt'
  if ([45, 48].includes(code)) return 'Nebel'
  if ([51, 53, 55, 56, 57].includes(code)) return 'Nieselregen'
  if ([61, 63, 65, 66, 67].includes(code)) return 'Regen'
  if ([71, 73, 75, 77, 85, 86].includes(code)) return 'Schnee'
  if ([80, 81, 82].includes(code)) return 'Regenschauer'
  if ([95, 96, 99].includes(code)) return 'Gewitter'
  return 'Wechselhaft'
}

function weatherSymbol(code: number) {
  if (code === 0) return '☀'
  if ([1, 2].includes(code)) return '◒'
  if (code === 3) return '☁'
  if ([45, 48].includes(code)) return '≋'
  if ([71, 73, 75, 77, 85, 86].includes(code)) return '❄'
  if ([95, 96, 99].includes(code)) return 'ϟ'
  if ([51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 80, 81, 82].includes(code)) return '☂'
  return '◌'
}

function confidenceLabel(value: WeatherConsensus['confidence']) {
  if (value === 'high') return 'Hohe Übereinstimmung'
  if (value === 'medium') return 'Mittlere Übereinstimmung'
  return 'Modelle uneinig'
}

function sourceLabel(value: WeatherForecast['source']) {
  if (value === 'cache') return 'Cache'
  if (value === 'stale') return 'letzter verfügbarer Stand'
  return 'frisch geladen'
}

function currentIndex(model: WeatherModel) {
  if (!forecast.value) {
    return 0
  }

  const index = model.hourly.findIndex((point) => point.time >= forecast.value!.current.time)
  return index === -1 ? 0 : index
}

const nextSixHours = computed(() => {
  if (!forecast.value || forecast.value.models.length === 0) {
    return []
  }

  const firstModel = forecast.value.models[0]!
  const start = currentIndex(firstModel)

  return firstModel.hourly.slice(start, start + 6).map((point, offset) => {
    const probabilities = forecast
      .value!.models.map((model) => model.hourly[start + offset]?.precipitationProbability)
      .filter((value): value is number => value !== undefined)

    return {
      time: point.time,
      probability: median(probabilities),
      agreement: probabilities.filter((value) => value >= 40).length,
    }
  })
})

const dailyConsensus = computed(() => {
  if (!forecast.value || forecast.value.models.length === 0) {
    return []
  }

  return forecast.value.models[0]!.daily.map((day, index) => {
    const modelDays = forecast
      .value!.models.map((model) => model.daily[index])
      .filter((value): value is WeatherDailyPoint => value !== undefined)
    const maximums = modelDays.map((value) => value.temperatureMax)

    return {
      date: day.date,
      code: day.weatherCode,
      minimum: median(modelDays.map((value) => value.temperatureMin)),
      maximum: median(maximums),
      maximumMin: Math.min(...maximums),
      maximumMax: Math.max(...maximums),
      rain: median(modelDays.map((value) => value.precipitationProbability)),
    }
  })
})

const temperatureChart = computed(() => {
  if (!forecast.value || forecast.value.models.length === 0) {
    return null
  }

  const width = 760
  const height = 230
  const paddingX = 34
  const paddingY = 24
  const firstModel = forecast.value.models[0]!
  const start = currentIndex(firstModel)
  const count = 24
  const visibleModels = forecast.value.models.map((model) => ({
    ...model,
    points: model.hourly.slice(start, start + count),
  }))
  const temperatures = visibleModels.flatMap((model) =>
    model.points.map((point) => point.temperature),
  )

  if (temperatures.length === 0) {
    return null
  }

  const min = Math.floor(Math.min(...temperatures) - 1)
  const max = Math.ceil(Math.max(...temperatures) + 1)
  const range = Math.max(max - min, 1)
  const drawableWidth = width - paddingX * 2
  const drawableHeight = height - paddingY * 2

  const paths = visibleModels.map((model) => ({
    id: model.id,
    name: model.name,
    short: model.short,
    color: model.color,
    points: model.points
      .map((point, index) => {
        const x = paddingX + (index / Math.max(model.points.length - 1, 1)) * drawableWidth
        const y = paddingY + ((max - point.temperature) / range) * drawableHeight
        return `${x.toFixed(1)},${y.toFixed(1)}`
      })
      .join(' '),
  }))

  const labelIndexes = [0, 6, 12, 18, 23].filter((index) => index < visibleModels[0]!.points.length)
  const labels = labelIndexes.map((index) => ({
    x: paddingX + (index / Math.max(visibleModels[0]!.points.length - 1, 1)) * drawableWidth,
    text: formatHour(visibleModels[0]!.points[index]!.time),
  }))

  return { width, height, paddingX, paddingY, min, max, paths, labels }
})

async function loadWeather(refresh = false) {
  if (refresh) {
    refreshing.value = true
  } else {
    loading.value = true
  }
  error.value = ''

  try {
    forecast.value = await apiRequest<WeatherForecast>(
      refresh ? '/api/weather/refresh' : '/api/weather',
      refresh ? { method: 'POST' } : {},
    )
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Wetter konnte nicht geladen werden'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function searchLocations() {
  const query = locationQuery.value.trim()
  if (query.length < 2) {
    locationError.value = 'Bitte mindestens zwei Zeichen eingeben.'
    return
  }

  searching.value = true
  locationError.value = ''
  locationResults.value = []

  try {
    const data = await apiRequest<LocationSearchResponse>(
      `/api/weather/locations?q=${encodeURIComponent(query)}`,
    )
    locationResults.value = data.locations

    if (data.locations.length === 0) {
      locationError.value = 'Kein passender Ort gefunden.'
    }
  } catch (err) {
    locationError.value =
      err instanceof Error ? err.message : 'Ortssuche konnte nicht geladen werden'
  } finally {
    searching.value = false
  }
}

async function selectLocation(location: WeatherLocation) {
  savingLocation.value = true
  locationError.value = ''

  try {
    await apiRequest<{ location: WeatherLocation }>('/api/weather/location', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(location),
    })

    showSettings.value = false
    locationResults.value = []
    await loadWeather()
  } catch (err) {
    locationError.value = err instanceof Error ? err.message : 'Ort konnte nicht gespeichert werden'
  } finally {
    savingLocation.value = false
  }
}

onMounted(loadWeather)
</script>

<template>
  <section class="weather-panel">
    <header class="weather-header">
      <div>
        <p class="eyebrow">FORECAST LAB</p>
        <h2>Wettermodelle</h2>
        <button
          v-if="forecast"
          class="location-button"
          type="button"
          @click="showSettings = !showSettings"
        >
          {{ forecast.location.name }}
          <span v-if="forecast.location.admin1">· {{ forecast.location.admin1 }}</span>
        </button>
      </div>

      <button
        class="refresh-button"
        type="button"
        :disabled="loading || refreshing"
        @click="loadWeather(true)"
      >
        {{ refreshing ? 'Lädt …' : 'Refresh' }}
      </button>
    </header>

    <form v-if="showSettings" class="location-settings" @submit.prevent="searchLocations">
      <div>
        <strong>Wetterort ändern</strong>
        <p>Stadt, Ortsteil oder Postleitzahl suchen.</p>
      </div>

      <div class="location-search">
        <input v-model="locationQuery" aria-label="Wetterort" placeholder="z. B. 51069 oder Köln" />
        <button type="submit" :disabled="searching">
          {{ searching ? 'Suche …' : 'Suchen' }}
        </button>
      </div>

      <p v-if="locationError" class="error-text">{{ locationError }}</p>

      <div v-if="locationResults.length" class="location-results">
        <button
          v-for="location in locationResults"
          :key="`${location.latitude}:${location.longitude}`"
          type="button"
          :disabled="savingLocation"
          @click="selectLocation(location)"
        >
          <strong>{{ location.name }}</strong>
          <span>{{ [location.admin1, location.country].filter(Boolean).join(' · ') }}</span>
        </button>
      </div>
    </form>

    <div v-if="loading" class="state-card">Hermes startet die Wettermodelle …</div>
    <div v-else-if="error" class="state-card error-text">{{ error }}</div>

    <template v-else-if="forecast">
      <section class="current-grid">
        <article class="current-card">
          <div class="weather-symbol">{{ weatherSymbol(forecast.current.weatherCode) }}</div>
          <div>
            <p class="current-temperature">
              {{ formatTemperature(forecast.current.temperature) }}
            </p>
            <p>{{ weatherLabel(forecast.current.weatherCode) }}</p>
            <p class="muted-text">
              Gefühlt {{ formatTemperature(forecast.current.apparentTemperature) }} · Wind
              {{ Math.round(forecast.current.windSpeed) }} km/h
            </p>
          </div>
        </article>

        <article class="consensus-card">
          <div class="consensus-top">
            <div>
              <span class="confidence-dot" :class="`confidence-${forecast.consensus.confidence}`" />
              {{ confidenceLabel(forecast.consensus.confidence) }}
            </div>
            <strong>{{ formatTemperature(forecast.consensus.todayMaxMedian) }}</strong>
          </div>
          <p>
            Tagesmaximum zwischen
            {{ formatTemperature(forecast.consensus.todayMaxMin) }} und
            {{ formatTemperature(forecast.consensus.todayMaxMax) }}.
          </p>
          <p>
            Regen nächste 6 Stunden:
            <strong>{{ Math.round(forecast.consensus.rainProbabilityNext6) }} %</strong>
            · {{ forecast.consensus.rainAgreementNext6 }}/3 Modelle
          </p>
        </article>
      </section>

      <aside
        v-if="forecast.consensus.heatLevel !== 'normal'"
        class="heat-notice"
        :class="`heat-${forecast.consensus.heatLevel}`"
      >
        <strong>Hermes-Hitzeindikator</strong>
        <span>{{ forecast.consensus.heatMessage }} Keine amtliche DWD-Warnung.</span>
      </aside>

      <section class="rain-strip">
        <div class="section-heading">
          <div>
            <p class="section-label">NÄCHSTE 6 STUNDEN</p>
            <h3>Regenkonsens</h3>
          </div>
          <span v-if="forecast.consensus.rainStart" class="signal">
            Signal ab {{ formatHour(forecast.consensus.rainStart) }}
          </span>
        </div>

        <div class="rain-hours">
          <article v-for="hour in nextSixHours" :key="hour.time">
            <time>{{ formatHour(hour.time) }}</time>
            <strong>{{ Math.round(hour.probability) }} %</strong>
            <span>{{ hour.agreement }}/3 Modelle</span>
            <div class="rain-meter">
              <i :style="{ width: `${hour.probability}%` }" />
            </div>
          </article>
        </div>
      </section>

      <section v-if="temperatureChart" class="chart-card">
        <div class="section-heading">
          <div>
            <p class="section-label">24-STUNDEN-VERGLEICH</p>
            <h3>Temperaturkurven</h3>
          </div>
          <div class="legend">
            <span v-for="path in temperatureChart.paths" :key="path.id">
              <i :style="{ background: path.color }" />
              {{ path.short }}
            </span>
          </div>
        </div>

        <svg
          class="temperature-chart"
          :viewBox="`0 0 ${temperatureChart.width} ${temperatureChart.height}`"
          role="img"
          aria-label="Temperaturvergleich der Wettermodelle"
        >
          <line
            :x1="temperatureChart.paddingX"
            :x2="temperatureChart.width - temperatureChart.paddingX"
            :y1="temperatureChart.paddingY"
            :y2="temperatureChart.paddingY"
            class="grid-line"
          />
          <line
            :x1="temperatureChart.paddingX"
            :x2="temperatureChart.width - temperatureChart.paddingX"
            :y1="temperatureChart.height - temperatureChart.paddingY"
            :y2="temperatureChart.height - temperatureChart.paddingY"
            class="grid-line"
          />

          <text x="2" :y="temperatureChart.paddingY + 4" class="axis-label">
            {{ temperatureChart.max }}°
          </text>
          <text
            x="2"
            :y="temperatureChart.height - temperatureChart.paddingY + 4"
            class="axis-label"
          >
            {{ temperatureChart.min }}°
          </text>

          <polyline
            v-for="path in temperatureChart.paths"
            :key="path.id"
            :points="path.points"
            :stroke="path.color"
            class="model-line"
          />

          <g v-for="label in temperatureChart.labels" :key="label.text">
            <line
              :x1="label.x"
              :x2="label.x"
              :y1="temperatureChart.paddingY"
              :y2="temperatureChart.height - temperatureChart.paddingY"
              class="time-line"
            />
            <text
              :x="label.x"
              :y="temperatureChart.height - 4"
              text-anchor="middle"
              class="axis-label"
            >
              {{ label.text }}
            </text>
          </g>
        </svg>
      </section>

      <section class="model-grid">
        <article
          v-for="model in forecast.models"
          :key="model.id"
          class="model-card"
          :style="{ '--model-color': model.color }"
        >
          <div class="model-name">
            <i />
            <strong>{{ model.name }}</strong>
          </div>
          <p v-if="model.daily[0]">
            Heute {{ formatTemperature(model.daily[0].temperatureMax) }}
            <span>· Regen {{ Math.round(model.daily[0].precipitationProbability) }} %</span>
          </p>
          <p v-if="model.daily[1]" class="muted-text">
            Morgen {{ formatTemperature(model.daily[1].temperatureMax) }}
          </p>
        </article>
      </section>

      <section class="daily-card">
        <div class="section-heading">
          <div>
            <p class="section-label">MODELL-MEDIAN</p>
            <h3>Fünf Tage</h3>
          </div>
        </div>

        <div class="daily-list">
          <article v-for="day in dailyConsensus" :key="day.date">
            <time>{{ formatDay(day.date) }}</time>
            <span class="daily-symbol">{{ weatherSymbol(day.code) }}</span>
            <strong>
              {{ formatTemperature(day.minimum) }} /
              {{ formatTemperature(day.maximum) }}
            </strong>
            <span>{{ Math.round(day.rain) }} % Regen</span>
            <small>
              Modelle: {{ formatTemperature(day.maximumMin) }}–{{
                formatTemperature(day.maximumMax)
              }}
            </small>
          </article>
        </div>
      </section>

      <footer class="weather-footer">
        <span>
          Aktualisiert {{ formatRefreshedAt(forecast.refreshedAt) }} ·
          {{ sourceLabel(forecast.source) }}
        </span>
        <a href="https://open-meteo.com/" target="_blank" rel="noopener noreferrer">
          Daten: Open-Meteo · DWD · ECMWF · NOAA
        </a>
      </footer>
    </template>
  </section>
</template>

<style scoped>
.weather-panel {
  display: grid;
  gap: 1rem;
}

.weather-header,
.section-heading,
.consensus-top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.eyebrow,
.section-label {
  margin: 0 0 0.35rem;
  color: #5ee6a8;
  font-family: monospace;
  font-size: 0.72rem;
  letter-spacing: 0.15em;
}

h2,
h3,
p {
  margin: 0;
}

h2 {
  font-size: 2rem;
}

h3 {
  font-size: 1rem;
}

button,
input {
  font: inherit;
}

button {
  cursor: pointer;
}

button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.location-button {
  margin-top: 0.35rem;
  padding: 0;
  border: 0;
  background: transparent;
  color: #a9b6c4;
  text-align: left;
}

.location-button:hover {
  color: #5ee6a8;
}

.refresh-button,
.location-search button {
  padding: 0.6rem 0.85rem;
  border: 1px solid #334155;
  border-radius: 10px;
  background: transparent;
  color: #edf4fa;
}

.location-settings,
.state-card,
.current-card,
.consensus-card,
.rain-strip,
.chart-card,
.model-card,
.daily-card {
  border: 1px solid #273342;
  border-radius: 18px;
  background: rgba(15, 23, 32, 0.86);
}

.location-settings {
  display: grid;
  gap: 0.9rem;
  padding: 1rem;
}

.location-settings p {
  margin-top: 0.25rem;
  color: #7f8ea3;
  font-size: 0.85rem;
}

.location-search {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.6rem;
}

.location-search input {
  min-width: 0;
  padding: 0.7rem 0.85rem;
  border: 1px solid #334155;
  border-radius: 10px;
  background: #0b1119;
  color: #edf4fa;
}

.location-results {
  display: grid;
  gap: 0.5rem;
}

.location-results button {
  display: grid;
  gap: 0.15rem;
  padding: 0.7rem 0.8rem;
  border: 1px solid #334155;
  border-radius: 10px;
  background: #0b1119;
  color: #edf4fa;
  text-align: left;
}

.location-results button:hover {
  border-color: #5ee6a8;
}

.location-results span {
  color: #7f8ea3;
  font-size: 0.8rem;
}

.state-card {
  padding: 1.2rem;
  color: #a9b6c4;
}

.current-grid {
  display: grid;
  grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr);
  gap: 1rem;
}

.current-card,
.consensus-card {
  padding: 1.15rem;
}

.current-card {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.weather-symbol {
  display: grid;
  width: 62px;
  height: 62px;
  place-items: center;
  border: 1px solid rgba(94, 230, 168, 0.35);
  border-radius: 18px;
  background: rgba(94, 230, 168, 0.07);
  color: #5ee6a8;
  font-size: 2rem;
}

.current-temperature {
  font-size: 2.5rem;
  font-weight: 750;
  line-height: 1;
}

.muted-text {
  color: #7f8ea3;
  font-size: 0.82rem;
}

.consensus-card {
  display: grid;
  gap: 0.55rem;
}

.consensus-top div {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  color: #c8d2dc;
  font-size: 0.85rem;
}

.consensus-top > strong {
  font-size: 1.7rem;
}

.consensus-card > p {
  color: #a9b6c4;
  line-height: 1.45;
}

.confidence-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
}

.confidence-high {
  background: #5ee6a8;
  box-shadow: 0 0 10px rgba(94, 230, 168, 0.65);
}

.confidence-medium {
  background: #ffbd6e;
}

.confidence-low {
  background: #ff7d91;
}

.heat-notice {
  display: grid;
  gap: 0.25rem;
  padding: 0.85rem 1rem;
  border: 1px solid rgba(255, 189, 110, 0.45);
  border-radius: 14px;
  background: rgba(255, 189, 110, 0.08);
}

.heat-danger {
  border-color: rgba(255, 86, 111, 0.55);
  background: rgba(255, 86, 111, 0.08);
}

.heat-notice span {
  color: #c8d2dc;
  font-size: 0.85rem;
}

.rain-strip,
.chart-card,
.daily-card {
  padding: 1rem;
}

.signal {
  color: #7aa2ff;
  font-size: 0.8rem;
}

.rain-hours {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 0.5rem;
  margin-top: 0.8rem;
}

.rain-hours article {
  display: grid;
  gap: 0.25rem;
  min-width: 0;
  padding: 0.65rem;
  border: 1px solid #273342;
  border-radius: 12px;
  background: #0b1119;
}

.rain-hours time,
.rain-hours span {
  color: #7f8ea3;
  font-size: 0.72rem;
}

.rain-hours strong {
  font-size: 1.05rem;
}

.rain-meter {
  height: 3px;
  overflow: hidden;
  border-radius: 99px;
  background: #273342;
}

.rain-meter i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: #7aa2ff;
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 0.7rem;
}

.legend span {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  color: #a9b6c4;
  font-size: 0.75rem;
}

.legend i,
.model-name i {
  display: inline-block;
  width: 9px;
  height: 9px;
  border-radius: 50%;
}

.temperature-chart {
  display: block;
  width: 100%;
  margin-top: 0.7rem;
  overflow: visible;
}

.model-line {
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 3;
}

.grid-line,
.time-line {
  stroke: #273342;
  stroke-width: 1;
}

.time-line {
  stroke-dasharray: 3 5;
  opacity: 0.6;
}

.axis-label {
  fill: #7f8ea3;
  font-family: monospace;
  font-size: 11px;
}

.model-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.7rem;
}

.model-card {
  display: grid;
  gap: 0.4rem;
  padding: 0.85rem;
  border-top-color: var(--model-color);
}

.model-name {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.model-name i {
  background: var(--model-color);
}

.model-card p {
  color: #c8d2dc;
  font-size: 0.85rem;
}

.model-card p span {
  color: #7f8ea3;
}

.daily-list {
  display: grid;
  gap: 0.25rem;
  margin-top: 0.7rem;
}

.daily-list article {
  display: grid;
  grid-template-columns: 86px 28px 100px 100px minmax(0, 1fr);
  gap: 0.5rem;
  align-items: center;
  padding: 0.65rem 0;
  border-top: 1px solid #273342;
}

.daily-list time,
.daily-list span,
.daily-list small {
  color: #a9b6c4;
  font-size: 0.8rem;
}

.daily-symbol {
  color: #5ee6a8 !important;
  font-size: 1.1rem !important;
}

.weather-footer {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 0.7rem;
  color: #66758a;
  font-size: 0.72rem;
}

.weather-footer a {
  color: #7f8ea3;
}

.error-text {
  color: #ff9baa !important;
}

@media (max-width: 760px) {
  .current-grid,
  .model-grid {
    grid-template-columns: 1fr;
  }

  .rain-hours {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .daily-list article {
    grid-template-columns: 72px 24px 90px minmax(0, 1fr);
  }

  .daily-list small {
    display: none;
  }
}

@media (max-width: 480px) {
  .weather-header,
  .section-heading {
    display: grid;
  }

  .rain-hours {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .location-search {
    grid-template-columns: 1fr;
  }
}
</style>
