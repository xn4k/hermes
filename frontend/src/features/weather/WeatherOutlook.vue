<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { apiRequest } from '../../api'

type View = '16' | '30'
type Source = 'refresh' | 'cache' | 'stale'
type ModelDay = {
  date: string
  temperatureMin: number
  temperatureMax: number
  precipitationProbability: number | null
  precipitation: number
}
type Model = {
  id: string
  name: string
  short: string
  color: string
  horizonDays: number
  daily: ModelDay[]
}
type EnsembleDay = {
  date: string
  temperatureMedian: number
  temperatureP10: number
  temperatureP90: number
  precipitationMedian: number
  precipitationP10: number
  precipitationP90: number
}
type Ensemble = {
  id: string
  name: string
  short: string
  color: string
  memberCount: number
  daily: EnsembleDay[]
}
type Outlook = {
  mode: 'models' | 'ensemble'
  horizonDays: number
  models?: Model[]
  ensembles?: Ensemble[]
  notice: string
  refreshedAt: string
  source: Source
}
type RawSeries = {
  id: string
  label: string
  color: string
  values: number[]
  lower?: number[]
  upper?: number[]
  dashed?: boolean
}

const view = ref<View | null>(null)
const data = ref<Outlook | null>(null)
const loading = ref(false)
const refreshing = ref(false)
const error = ref('')
const shortMetric = ref<'max' | 'min'>('max')
const longMetric = ref<'temperature' | 'rain'>('temperature')
const hoverIndex = ref<number | null>(null)
const activeSeriesId = ref<string | null>(null)

function median(values: number[]) {
  if (!values.length) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[middle]! : (sorted[middle - 1]! + sorted[middle]!) / 2
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('de-DE', { day: '2-digit', month: '2-digit' }).format(
    new Date(`${value}T12:00:00`),
  )
}

function formatLongDate(value: string) {
  return new Intl.DateTimeFormat('de-DE', {
    weekday: 'long',
    day: '2-digit',
    month: 'long',
  }).format(new Date(`${value}T12:00:00`))
}

function formatValue(value: number, unit: string) {
  return `${value.toFixed(1).replace('.', ',')}${unit}`
}

function formatRefresh(value: string) {
  return new Intl.DateTimeFormat('de-DE', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function sourceLabel(source: Source) {
  if (source === 'cache') return 'Cache'
  if (source === 'stale') return 'letzter verfügbarer Stand'
  return 'frisch geladen'
}

const dates = computed(() => {
  if (data.value?.mode === 'models') {
    return [
      ...new Set(data.value.models?.flatMap((model) => model.daily.map((day) => day.date)) ?? []),
    ].sort()
  }
  return data.value?.ensembles?.[0]?.daily.map((day) => day.date) ?? []
})

const rawSeries = computed<RawSeries[]>(() => {
  if (data.value?.mode === 'models') {
    const models = data.value.models ?? []
    const lines = models.map((model) => ({
      id: model.id,
      label: model.short,
      color: model.color,
      values: dates.value.map((date) => {
        const day = model.daily.find((item) => item.date === date)
        return day ? (shortMetric.value === 'max' ? day.temperatureMax : day.temperatureMin) : NaN
      }),
    }))
    const lows: number[] = []
    const highs: number[] = []
    const medians: number[] = []
    dates.value.forEach((_, index) => {
      const values = lines.map((line) => line.values[index]!).filter(Number.isFinite)
      lows.push(Math.min(...values))
      highs.push(Math.max(...values))
      medians.push(median(values))
    })
    return [
      {
        id: 'model-span',
        label: 'Modellspanne',
        color: '#5ee6a8',
        values: medians,
        lower: lows,
        upper: highs,
        dashed: true,
      },
      ...lines,
    ]
  }

  return (data.value?.ensembles ?? []).map((model) => {
    const dayFor = (date: string) => model.daily.find((day) => day.date === date)
    return {
      id: model.id,
      label: model.short,
      color: model.color,
      values: dates.value.map((date) => {
        const day = dayFor(date)
        if (!day) return NaN
        return longMetric.value === 'temperature'
          ? day.temperatureMedian
          : day.precipitationMedian
      }),
      lower: dates.value.map((date) => {
        const day = dayFor(date)
        if (!day) return NaN
        return longMetric.value === 'temperature' ? day.temperatureP10 : day.precipitationP10
      }),
      upper: dates.value.map((date) => {
        const day = dayFor(date)
        if (!day) return NaN
        return longMetric.value === 'temperature' ? day.temperatureP90 : day.precipitationP90
      }),
    }
  })
})

const chart = computed(() => {
  if (!dates.value.length || !rawSeries.value.length) return null
  const width = 900
  const height = 310
  const paddingX = 54
  const paddingY = 34
  const allValues = rawSeries.value.flatMap((series) => [
    ...series.values.filter(Number.isFinite),
    ...(series.lower?.filter(Number.isFinite) ?? []),
    ...(series.upper?.filter(Number.isFinite) ?? []),
  ])
  const rain = data.value?.mode === 'ensemble' && longMetric.value === 'rain'
  const rawMin = Math.min(...allValues)
  const rawMax = Math.max(...allValues)
  const margin = Math.max((rawMax - rawMin) * 0.08, rain ? 0.4 : 1)
  const minimum = rain ? 0 : Math.floor(rawMin - margin)
  const maximum = Math.ceil(rawMax + margin)
  const range = Math.max(maximum - minimum, 1)
  const drawWidth = width - paddingX * 2
  const drawHeight = height - paddingY * 2
  const x = (index: number) => paddingX + (index / Math.max(dates.value.length - 1, 1)) * drawWidth
  const y = (value: number) => paddingY + ((maximum - value) / range) * drawHeight
  const points = (values: number[]) =>
    values
      .map((value, index) =>
        Number.isFinite(value) ? `${x(index).toFixed(1)},${y(value).toFixed(1)}` : '',
      )
      .filter(Boolean)
      .join(' ')
  const series = rawSeries.value.map((item) => {
    const bandIndexes =
      item.lower
        ?.map((lower, index) => ({ index, lower, upper: item.upper?.[index] }))
        .filter((point) => Number.isFinite(point.lower) && Number.isFinite(point.upper)) ?? []
    const upper = bandIndexes.map(
      (point) => `${x(point.index).toFixed(1)},${y(point.upper!).toFixed(1)}`,
    )
    const lower = bandIndexes
      .map((point) => `${x(point.index).toFixed(1)},${y(point.lower).toFixed(1)}`)
      .reverse()
    return {
      ...item,
      points: points(item.values),
      band: [...upper, ...lower].join(' '),
      samples: item.values.map((value, index) =>
        Number.isFinite(value) ? { x: x(index), y: y(value), value } : null,
      ),
    }
  })
  const tickCount = dates.value.length > 20 ? 6 : 5
  const ticks = Array.from({ length: tickCount }, (_, index) =>
    Math.round((index / (tickCount - 1)) * (dates.value.length - 1)),
  ).map((index) => ({ x: x(index), label: formatDate(dates.value[index]!) }))

  return {
    width,
    height,
    paddingX,
    paddingY,
    minimum,
    maximum,
    unit: rain ? 'mm' : '°',
    series,
    ticks,
    xPositions: dates.value.map((_, index) => x(index)),
  }
})

const hover = computed(() => {
  const index = hoverIndex.value
  const currentChart = chart.value
  if (index === null || !currentChart || !dates.value[index]) return null

  const rows = currentChart.series.flatMap((series) => {
    const sample = series.samples[index]
    if (!sample) return []
    const lower = series.lower?.[index]
    const upper = series.upper?.[index]
    return [{
      id: series.id,
      label: series.id === 'model-span' ? 'Median' : series.label,
      color: series.color,
      value: sample.value,
      lower: Number.isFinite(lower) ? lower! : null,
      upper: Number.isFinite(upper) ? upper! : null,
    }]
  })

  return {
    index,
    date: dates.value[index]!,
    x: currentChart.xPositions[index]!,
    rows,
  }
})

function updateChartHover(event: PointerEvent) {
  const currentChart = chart.value
  if (!currentChart) return
  const svg = event.currentTarget as SVGSVGElement
  const bounds = svg.getBoundingClientRect()
  if (!bounds.width) return
  const viewX = ((event.clientX - bounds.left) / bounds.width) * currentChart.width
  const drawWidth = currentChart.width - currentChart.paddingX * 2
  const ratio = Math.min(
    1,
    Math.max(0, (viewX - currentChart.paddingX) / Math.max(drawWidth, 1)),
  )
  hoverIndex.value = Math.round(ratio * Math.max(dates.value.length - 1, 0))
}

function clearChartHover(event: PointerEvent) {
  if (event.pointerType !== 'touch') {
    hoverIndex.value = null
    activeSeriesId.value = null
  }
}

function moveChartHover(direction: number) {
  if (!dates.value.length) return
  const start = hoverIndex.value ?? (direction > 0 ? -1 : dates.value.length)
  hoverIndex.value = Math.min(dates.value.length - 1, Math.max(0, start + direction))
}

watch([view, shortMetric, longMetric], () => {
  hoverIndex.value = null
  activeSeriesId.value = null
})
async function load(nextView: View, refresh = false) {
  view.value = nextView
  hoverIndex.value = null
  activeSeriesId.value = null
  error.value = ''
  if (refresh) refreshing.value = true
  else loading.value = true

  try {
    data.value = await apiRequest<Outlook>(
      refresh ? `/api/weather/refresh?view=${nextView}` : `/api/weather?view=${nextView}`,
      refresh ? { method: 'POST' } : {},
    )
  } catch (reason) {
    data.value = null
    error.value =
      reason instanceof Error ? reason.message : 'Langfristprognose konnte nicht geladen werden'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}
</script>

<template>
  <section class="outlook">
    <header class="outlook-header">
      <div>
        <p class="label">HERMES WEITBLICK</p>
        <h3>Langfrist-Lab</h3>
        <span>Modellvergleich und Ensembletrends werden erst bei Bedarf geladen.</span>
      </div>
      <div class="view-switch">
        <button :class="{ active: view === '16' }" :disabled="loading" @click="load('16')">
          16 Tage
        </button>
        <button :class="{ active: view === '30' }" :disabled="loading" @click="load('30')">
          30 Tage
        </button>
      </div>
    </header>

    <div v-if="!view" class="teasers">
      <article><strong>16 Tage</strong><span>ICON · IFS · AIFS · GFS</span></article>
      <article><strong>30 Tage</strong><span>82 Ensemble-Szenarien · P10–P90</span></article>
    </div>
    <div v-else-if="loading" class="state">Hermes sammelt die Langfristläufe …</div>
    <div v-else-if="error" class="state error">{{ error }}</div>

    <template v-else-if="data && chart">
      <div class="notice">
        <p>{{ data.notice }}</p>
        <button :disabled="refreshing" @click="load(view!, true)">
          {{ refreshing ? 'Aktualisiert …' : 'Neu rechnen' }}
        </button>
      </div>

      <div class="metric-switch">
        <template v-if="data.mode === 'models'">
          <button :class="{ active: shortMetric === 'max' }" @click="shortMetric = 'max'">
            Höchstwerte
          </button>
          <button :class="{ active: shortMetric === 'min' }" @click="shortMetric = 'min'">
            Tiefstwerte
          </button>
        </template>
        <template v-else>
          <button
            :class="{ active: longMetric === 'temperature' }"
            @click="longMetric = 'temperature'"
          >
            Temperatur
          </button>
          <button :class="{ active: longMetric === 'rain' }" @click="longMetric = 'rain'">
            Niederschlag
          </button>
        </template>
      </div>

      <section class="chart-card">
        <div class="chart-heading">
          <div>
            <p class="label">{{ data.mode === 'models' ? 'MODELLVERGLEICH' : 'ENSEMBLETREND' }}</p>
            <h4>
              {{ data.horizonDays }} Tage · {{ data.mode === 'models' ? 'Einzelläufe' : 'P10–P90' }}
            </h4>
          </div>
          <div class="legend">
            <span v-for="series in chart.series" :key="series.id">
              <i :style="{ background: series.color }" />{{ series.label }}
            </span>
          </div>
        </div>

        <svg
          class="chart"
          :viewBox="`0 0 ${chart.width} ${chart.height}`"
          role="img"
          :aria-label="`${data.horizonDays}-Tage-Wettervergleich`"
          tabindex="0"
          @pointerdown="updateChartHover"
          @pointermove="updateChartHover"
          @pointerleave="clearChartHover"
          @keydown.left.prevent="moveChartHover(-1)"
          @keydown.right.prevent="moveChartHover(1)"
        >
          <line
            :x1="chart.paddingX"
            :x2="chart.width - chart.paddingX"
            :y1="chart.paddingY"
            :y2="chart.paddingY"
            class="grid"
          />
          <line
            :x1="chart.paddingX"
            :x2="chart.width - chart.paddingX"
            :y1="chart.height - chart.paddingY"
            :y2="chart.height - chart.paddingY"
            class="grid"
          />
          <polygon
            v-for="series in chart.series.filter((item) => item.band)"
            :key="`${series.id}-band`"
            :points="series.band"
            :fill="series.color"
            class="band"
            :class="{ dimmed: activeSeriesId !== null && activeSeriesId !== series.id }"
          />
          <polyline
            v-for="series in chart.series"
            :key="series.id"
            :points="series.points"
            :stroke="series.color"
            :class="[
              'line',
              {
                dashed: series.dashed,
                highlighted: activeSeriesId === series.id,
                dimmed: activeSeriesId !== null && activeSeriesId !== series.id,
              },
            ]"
          />
          <polyline
            v-for="series in chart.series"
            :key="`${series.id}-hit`"
            :points="series.points"
            class="line-hit"
            @pointerenter="activeSeriesId = series.id"
            @pointerleave="activeSeriesId = null"
          />
          <text x="4" :y="chart.paddingY + 4" class="axis">
            {{ chart.maximum }}{{ chart.unit }}
          </text>
          <text x="4" :y="chart.height - chart.paddingY + 4" class="axis">
            {{ chart.minimum }}{{ chart.unit }}
          </text>
          <g v-for="tick in chart.ticks" :key="tick.label">
            <line
              :x1="tick.x"
              :x2="tick.x"
              :y1="chart.paddingY"
              :y2="chart.height - chart.paddingY"
              class="time-grid"
            />
            <text :x="tick.x" :y="chart.height - 6" text-anchor="middle" class="axis">
              {{ tick.label }}
            </text>
          </g>

          <template v-if="hover">
            <line
              :x1="hover.x"
              :x2="hover.x"
              :y1="chart.paddingY"
              :y2="chart.height - chart.paddingY"
              class="hover-line"
            />
            <template v-for="series in chart.series" :key="`${series.id}-point`">
              <circle
                v-if="series.samples[hover.index]"
                :cx="series.samples[hover.index]?.x"
                :cy="series.samples[hover.index]?.y"
                r="5"
                :fill="series.color"
                class="hover-point"
              />
            </template>
            <foreignObject
              :x="hover.x > chart.width * 0.7 ? hover.x - 242 : hover.x + 12"
              :y="chart.paddingY + 6"
              width="230"
              :height="Math.min(42 + hover.rows.length * 31, chart.height - chart.paddingY * 2)"
              class="tooltip-object"
            >
              <div xmlns="http://www.w3.org/1999/xhtml" class="chart-tooltip">
                <strong>{{ formatLongDate(hover.date) }}</strong>
                <span
                  v-for="row in hover.rows"
                  :key="row.id"
                  class="tooltip-row"
                  :class="{ muted: activeSeriesId !== null && activeSeriesId !== row.id }"
                >
                  <i :style="{ background: row.color }" />
                  <span>{{ row.label }}</span>
                  <b>{{ formatValue(row.value, chart.unit) }}</b>
                  <small v-if="row.lower !== null && row.upper !== null">
                    {{ formatValue(row.lower, chart.unit) }}–{{
                      formatValue(row.upper, chart.unit)
                    }}
                  </small>
                </span>
              </div>
            </foreignObject>
          </template>
        </svg>
        <small v-if="data.mode === 'ensemble'">
          Linie = Median · Fläche = mittlere 80 % der Szenarien.
        </small>
      </section>

      <div v-if="data.mode === 'models'" class="cards models">
        <article v-for="model in data.models" :key="model.id" :style="{ '--color': model.color }">
          <strong>{{ model.name }}</strong
          ><span>{{ model.horizonDays }} Tage</span>
          <small>Linie endet mit der nativen Modellreichweite.</small>
        </article>
      </div>
      <div v-else class="cards">
        <article
          v-for="model in data.ensembles"
          :key="model.id"
          :style="{ '--color': model.color }"
        >
          <strong>{{ model.name }}</strong
          ><span>{{ model.memberCount }} Läufe</span>
          <small v-if="model.daily[29]">
            Tag 30: {{ Math.round(model.daily[29].temperatureMedian) }}° ·
            {{ Math.round(model.daily[29].temperatureP10) }}–{{
              Math.round(model.daily[29].temperatureP90)
            }}°
          </small>
        </article>
      </div>

      <footer>
        Aktualisiert {{ formatRefresh(data.refreshedAt) }} · {{ sourceLabel(data.source) }}
      </footer>
    </template>
  </section>
</template>

<style scoped>
.outlook {
  display: grid;
  gap: 0.85rem;
  padding: 1rem;
  border: 1px solid #273342;
  border-radius: 18px;
  background: radial-gradient(circle at 90% 0, #7aa2ff12, transparent 36%), #0f1720db;
}
.outlook-header,
.notice,
.chart-heading {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
}
h3,
h4,
p {
  margin: 0;
}
.outlook-header span,
.teasers span,
.notice p,
.chart-card small,
.cards small {
  color: #7f8ea3;
  font-size: 0.78rem;
}
.label {
  margin-bottom: 0.25rem;
  color: #7aa2ff;
  font-family: monospace;
  font-size: 0.68rem;
  letter-spacing: 0.14em;
}
button {
  padding: 0.52rem 0.7rem;
  border: 1px solid #334155;
  border-radius: 9px;
  background: #0b1119;
  color: #a9b6c4;
  cursor: pointer;
  font: inherit;
}
button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
button.active {
  border-color: #7aa2ff;
  background: #7aa2ff1f;
  color: #edf4fa;
}
.view-switch,
.metric-switch,
.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}
.metric-switch {
  justify-content: flex-end;
}
.teasers,
.cards {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.6rem;
}
.teasers article,
.cards article,
.state,
.chart-card {
  padding: 0.8rem;
  border: 1px solid #273342;
  border-radius: 12px;
  background: #0b1119;
}
.teasers article,
.cards article {
  display: grid;
  gap: 0.25rem;
}
.models {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}
.cards article {
  border-top-color: var(--color);
}
.cards span {
  color: #c8d2dc;
  font-size: 0.82rem;
}
.state {
  color: #a9b6c4;
}
.error {
  color: #ff9baa;
}
.notice {
  align-items: center;
  padding: 0.7rem 0.8rem;
  border-left: 3px solid #7aa2ff;
  border-radius: 8px;
  background: #7aa2ff12;
}
.chart-heading {
  align-items: flex-start;
}
.legend {
  justify-content: flex-end;
}
.legend span {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  color: #a9b6c4;
  font-size: 0.72rem;
}
.legend i {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.chart {
  display: block;
  width: 100%;
  margin-top: 0.6rem;
  overflow: visible;
  outline: none;
  touch-action: pan-y;
  user-select: none;
}
.chart:focus-visible {
  filter: drop-shadow(0 0 5px #7aa2ff66);
}
.band {
  opacity: 0.12;
  transition: opacity 140ms ease;
}
.line {
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 3;
  transition: opacity 140ms ease, stroke-width 140ms ease;
}
.dashed {
  stroke-dasharray: 7 6;
  stroke-width: 2;
}
.line.highlighted {
  stroke-width: 5;
  filter: drop-shadow(0 0 4px currentColor);
}
.line.dimmed { opacity: 0.2; }
.band.dimmed { opacity: 0.035; }
.line-hit {
  fill: none;
  stroke: transparent;
  stroke-width: 16;
  pointer-events: stroke;
}
.hover-line {
  stroke: #dbeafe;
  stroke-dasharray: 3 4;
  stroke-width: 1.5;
  pointer-events: none;
}
.hover-point {
  stroke: #0b1119;
  stroke-width: 3;
  pointer-events: none;
}
.tooltip-object {
  overflow: visible;
  pointer-events: none;
}
.chart-tooltip {
  display: grid;
  gap: 0.3rem;
  padding: 0.55rem 0.65rem;
  border: 1px solid #43536a;
  border-radius: 10px;
  background: #080d14f2;
  box-shadow: 0 10px 30px #0009;
  color: #edf4fa;
  font-size: 12px;
  backdrop-filter: blur(8px);
}
.chart-tooltip > strong {
  padding-bottom: 0.2rem;
  border-bottom: 1px solid #273342;
}
.tooltip-row {
  display: grid;
  grid-template-columns: 8px minmax(42px, 1fr) auto;
  align-items: center;
  gap: 0.35rem;
  transition: opacity 140ms ease;
}
.tooltip-row i {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.tooltip-row b { font-variant-numeric: tabular-nums; }
.tooltip-row small {
  grid-column: 2 / -1;
  color: #8999ad;
  font-size: 10px;
}
.tooltip-row.muted { opacity: 0.28; }
.grid,
.time-grid {
  stroke: #273342;
  stroke-width: 1;
}
.time-grid {
  stroke-dasharray: 3 5;
  opacity: 0.55;
}
.axis {
  fill: #7f8ea3;
  font-family: monospace;
  font-size: 11px;
}
footer {
  color: #66758a;
  font-size: 0.7rem;
  text-align: right;
}
@media (max-width: 760px) {
  .outlook-header,
  .notice,
  .chart-heading {
    display: grid;
  }
  .models {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .legend {
    justify-content: flex-start;
  }
}
@media (max-width: 480px) {
  .teasers,
  .cards,
  .models {
    grid-template-columns: 1fr;
  }
}
</style>
