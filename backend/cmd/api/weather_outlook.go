package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"golang.org/x/sync/errgroup"
)

const (
	openMeteoEnsembleURL = "https://ensemble-api.open-meteo.com/v1/ensemble"
	openMeteoSeasonalURL = "https://seasonal-api.open-meteo.com/v1/seasonal"
)

type WeatherOutlookDailyPoint struct {
	Date                     string   `json:"date"`
	TemperatureMin           float64  `json:"temperatureMin"`
	TemperatureMax           float64  `json:"temperatureMax"`
	PrecipitationProbability *float64 `json:"precipitationProbability"`
	Precipitation            float64  `json:"precipitation"`
}

type WeatherOutlookModel struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	Short       string                     `json:"short"`
	Color       string                     `json:"color"`
	HorizonDays int                        `json:"horizonDays"`
	Daily       []WeatherOutlookDailyPoint `json:"daily"`
}

type WeatherEnsembleDailyPoint struct {
	Date                string  `json:"date"`
	TemperatureMedian   float64 `json:"temperatureMedian"`
	TemperatureP10      float64 `json:"temperatureP10"`
	TemperatureP90      float64 `json:"temperatureP90"`
	PrecipitationMedian float64 `json:"precipitationMedian"`
	PrecipitationP10    float64 `json:"precipitationP10"`
	PrecipitationP90    float64 `json:"precipitationP90"`
}

type WeatherEnsembleModel struct {
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	Short       string                      `json:"short"`
	Color       string                      `json:"color"`
	MemberCount int                         `json:"memberCount"`
	Daily       []WeatherEnsembleDailyPoint `json:"daily"`
}

type WeatherOutlook struct {
	Location    WeatherLocation        `json:"location"`
	Mode        string                 `json:"mode"`
	HorizonDays int                    `json:"horizonDays"`
	Models      []WeatherOutlookModel  `json:"models,omitempty"`
	Ensembles   []WeatherEnsembleModel `json:"ensembles,omitempty"`
	Notice      string                 `json:"notice"`
	RefreshedAt time.Time              `json:"refreshedAt"`
	Source      string                 `json:"source"`
}

type openMeteoDynamicDailyResponse struct {
	Daily map[string]json.RawMessage `json:"daily"`
}

type weatherOutlookCacheEntry struct {
	outlook   WeatherOutlook
	expiresAt time.Time
}

var weatherOutlookCache = struct {
	sync.RWMutex
	entries map[string]weatherOutlookCacheEntry
}{
	entries: make(map[string]weatherOutlookCacheEntry),
}

func (app *App) handleWeatherOutlook(c *echo.Context, view string, force bool) error {
	if view != "16" && view != "30" {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid_weather_view"})
	}

	user, err := app.currentUser(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "not_authenticated"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 22*time.Second)
	defer cancel()

	location, err := app.weatherLocation(ctx, user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "weather_settings_failed"})
	}

	outlook, err := app.weather.outlook(ctx, location, view, force)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{
			"error":   "weather_fetch_failed",
			"details": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, outlook)
}

func (service *WeatherService) outlook(
	ctx context.Context,
	location WeatherLocation,
	view string,
	force bool,
) (WeatherOutlook, error) {
	key := weatherCacheKey(location) + ":outlook:" + view

	if !force {
		if cached, ok := cachedWeatherOutlook(key, false); ok {
			cached.Source = "cache"
			return cached, nil
		}
	}

	var (
		outlook WeatherOutlook
		err     error
	)

	switch view {
	case "16":
		outlook, err = service.fetchModelOutlook(ctx, location)
	case "30":
		outlook, err = service.fetchEnsembleOutlook(ctx, location)
	default:
		err = fmt.Errorf("unsupported weather outlook %q", view)
	}

	if err != nil {
		if cached, ok := cachedWeatherOutlook(key, true); ok {
			cached.Source = "stale"
			return cached, nil
		}

		return WeatherOutlook{}, err
	}

	weatherOutlookCache.Lock()
	weatherOutlookCache.entries[key] = weatherOutlookCacheEntry{
		outlook:   outlook,
		expiresAt: time.Now().Add(30 * time.Minute),
	}
	weatherOutlookCache.Unlock()

	return outlook, nil
}

func cachedWeatherOutlook(key string, allowExpired bool) (WeatherOutlook, bool) {
	weatherOutlookCache.RLock()
	entry, ok := weatherOutlookCache.entries[key]
	weatherOutlookCache.RUnlock()

	if !ok || (!allowExpired && time.Now().After(entry.expiresAt)) {
		return WeatherOutlook{}, false
	}

	return entry.outlook, true
}

func (service *WeatherService) fetchModelOutlook(
	ctx context.Context,
	location WeatherLocation,
) (WeatherOutlook, error) {
	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(location.Latitude, 'f', 5, 64))
	params.Set("longitude", strconv.FormatFloat(location.Longitude, 'f', 5, 64))
	params.Set("models", "icon_seamless,ecmwf_ifs025,ecmwf_aifs025_single,gfs_seamless")
	params.Set(
		"daily",
		"temperature_2m_max,temperature_2m_min,precipitation_probability_max,precipitation_sum",
	)
	params.Set("forecast_days", "16")
	params.Set("timezone", location.Timezone)

	daily, err := service.fetchDynamicDaily(ctx, openMeteoForecastURL, params)
	if err != nil {
		return WeatherOutlook{}, err
	}

	dates, err := decodeStringSeries(daily, "time")
	if err != nil {
		return WeatherOutlook{}, err
	}

	definitions := []outlookModelDefinition{
		{
			id:     "icon",
			name:   "DWD ICON",
			short:  "ICON",
			color:  "#5ee6a8",
			suffix: "icon_seamless",
		},
		{
			id:     "ecmwf",
			name:   "ECMWF IFS",
			short:  "IFS",
			color:  "#7aa2ff",
			suffix: "ecmwf_ifs025",
		},
		{
			id:     "aifs",
			name:   "ECMWF AIFS",
			short:  "AIFS",
			color:  "#c58cff",
			suffix: "ecmwf_aifs025_single",
		},
		{
			id:     "gfs",
			name:   "NOAA GFS",
			short:  "GFS",
			color:  "#ffbd6e",
			suffix: "gfs_seamless",
		},
	}

	models := make([]WeatherOutlookModel, 0, len(definitions))
	for _, definition := range definitions {
		model, modelErr := normalizeOutlookModel(daily, dates, definition)
		if modelErr != nil {
			return WeatherOutlook{}, modelErr
		}
		if len(model.Daily) > 0 {
			models = append(models, model)
		}
	}

	if len(models) < 2 {
		return WeatherOutlook{}, fmt.Errorf("model outlook returned only %d usable models", len(models))
	}

	return WeatherOutlook{
		Location:    location,
		Mode:        "models",
		HorizonDays: 16,
		Models:      models,
		Notice: "Die Linien enden bei der nativen Reichweite des jeweiligen Modells. " +
			"Ab Tag 8 sinkt die räumliche Präzision deutlich.",
		RefreshedAt: time.Now(),
		Source:      "refresh",
	}, nil
}

type outlookModelDefinition struct {
	id     string
	name   string
	short  string
	color  string
	suffix string
}

func normalizeOutlookModel(
	daily map[string]json.RawMessage,
	dates []string,
	definition outlookModelDefinition,
) (WeatherOutlookModel, error) {
	minimums, err := decodeNullableFloatSeries(
		daily,
		"temperature_2m_min_"+definition.suffix,
	)
	if err != nil {
		return WeatherOutlookModel{}, err
	}
	maximums, err := decodeNullableFloatSeries(
		daily,
		"temperature_2m_max_"+definition.suffix,
	)
	if err != nil {
		return WeatherOutlookModel{}, err
	}
	rainProbabilities, err := decodeNullableFloatSeries(
		daily,
		"precipitation_probability_max_"+definition.suffix,
	)
	if err != nil {
		return WeatherOutlookModel{}, err
	}
	precipitation, err := decodeNullableFloatSeries(
		daily,
		"precipitation_sum_"+definition.suffix,
	)
	if err != nil {
		return WeatherOutlookModel{}, err
	}

	points := make([]WeatherOutlookDailyPoint, 0, len(dates))
	for index, date := range dates {
		minimum := nullableValueAt(minimums, index)
		maximum := nullableValueAt(maximums, index)
		if minimum == nil || maximum == nil {
			continue
		}

		point := WeatherOutlookDailyPoint{
			Date:           date,
			TemperatureMin: *minimum,
			TemperatureMax: *maximum,
		}
		if value := nullableValueAt(rainProbabilities, index); value != nil {
			point.PrecipitationProbability = value
		}
		if value := nullableValueAt(precipitation, index); value != nil {
			point.Precipitation = *value
		}

		points = append(points, point)
	}

	return WeatherOutlookModel{
		ID:          definition.id,
		Name:        definition.name,
		Short:       definition.short,
		Color:       definition.color,
		HorizonDays: len(points),
		Daily:       points,
	}, nil
}

func (service *WeatherService) fetchEnsembleOutlook(
	ctx context.Context,
	location WeatherLocation,
) (WeatherOutlook, error) {
	group, groupContext := errgroup.WithContext(ctx)

	var gfs WeatherEnsembleModel
	var ec46 WeatherEnsembleModel

	group.Go(func() error {
		var err error
		gfs, err = service.fetchEnsembleModel(
			groupContext,
			openMeteoEnsembleURL,
			location,
			"ncep_gefs05",
			"gfs-ensemble",
			"NOAA GFS Ensemble",
			"GFS ENS",
			"#ffbd6e",
		)
		return err
	})

	group.Go(func() error {
		var err error
		ec46, err = service.fetchEnsembleModel(
			groupContext,
			openMeteoSeasonalURL,
			location,
			"ecmwf_ec46",
			"ec46",
			"ECMWF EC46",
			"EC46",
			"#7aa2ff",
		)
		return err
	})

	if err := group.Wait(); err != nil {
		return WeatherOutlook{}, err
	}

	return WeatherOutlook{
		Location:    location,
		Mode:        "ensemble",
		HorizonDays: 30,
		Ensembles:   []WeatherEnsembleModel{gfs, ec46},
		Notice: "30 Tage sind ein probabilistischer Trend. Das Band zeigt P10 bis P90 " +
			"der Ensembleläufe, keine garantierte Tagesvorhersage.",
		RefreshedAt: time.Now(),
		Source:      "refresh",
	}, nil
}

func (service *WeatherService) fetchEnsembleModel(
	ctx context.Context,
	endpoint string,
	location WeatherLocation,
	model string,
	id string,
	name string,
	short string,
	color string,
) (WeatherEnsembleModel, error) {
	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(location.Latitude, 'f', 5, 64))
	params.Set("longitude", strconv.FormatFloat(location.Longitude, 'f', 5, 64))
	params.Set("models", model)
	params.Set("daily", "temperature_2m_mean,precipitation_sum")
	params.Set("forecast_days", "30")
	params.Set("timezone", location.Timezone)

	daily, err := service.fetchDynamicDaily(ctx, endpoint, params)
	if err != nil {
		return WeatherEnsembleModel{}, fmt.Errorf("%s: %w", short, err)
	}

	dates, err := decodeStringSeries(daily, "time")
	if err != nil {
		return WeatherEnsembleModel{}, fmt.Errorf("%s: %w", short, err)
	}
	temperatureSeries, err := decodeMemberSeries(daily, "temperature_2m_mean")
	if err != nil {
		return WeatherEnsembleModel{}, fmt.Errorf("%s: %w", short, err)
	}
	precipitationSeries, err := decodeMemberSeries(daily, "precipitation_sum")
	if err != nil {
		return WeatherEnsembleModel{}, fmt.Errorf("%s: %w", short, err)
	}

	points := make([]WeatherEnsembleDailyPoint, 0, len(dates))
	for index, date := range dates {
		temperatures := memberValuesAt(temperatureSeries, index)
		if len(temperatures) == 0 {
			continue
		}
		precipitation := memberValuesAt(precipitationSeries, index)

		points = append(points, WeatherEnsembleDailyPoint{
			Date:                date,
			TemperatureMedian:   quantile(temperatures, 0.5),
			TemperatureP10:      quantile(temperatures, 0.1),
			TemperatureP90:      quantile(temperatures, 0.9),
			PrecipitationMedian: quantile(precipitation, 0.5),
			PrecipitationP10:    quantile(precipitation, 0.1),
			PrecipitationP90:    quantile(precipitation, 0.9),
		})
	}

	if len(points) == 0 {
		return WeatherEnsembleModel{}, fmt.Errorf("%s returned no daily ensemble data", short)
	}

	return WeatherEnsembleModel{
		ID:          id,
		Name:        name,
		Short:       short,
		Color:       color,
		MemberCount: len(temperatureSeries),
		Daily:       points,
	}, nil
}

func (service *WeatherService) fetchDynamicDaily(
	ctx context.Context,
	endpoint string,
	params url.Values,
) (map[string]json.RawMessage, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Hermes Dashboard/1.0")

	response, err := service.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("outlook returned HTTP %d", response.StatusCode)
	}

	var payload openMeteoDynamicDailyResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Daily) == 0 {
		return nil, fmt.Errorf("outlook returned no daily data")
	}

	return payload.Daily, nil
}

func decodeStringSeries(daily map[string]json.RawMessage, key string) ([]string, error) {
	raw, ok := daily[key]
	if !ok {
		return nil, fmt.Errorf("missing daily field %s", key)
	}

	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode %s: %w", key, err)
	}

	return values, nil
}

func decodeNullableFloatSeries(
	daily map[string]json.RawMessage,
	key string,
) ([]*float64, error) {
	raw, ok := daily[key]
	if !ok {
		return nil, fmt.Errorf("missing daily field %s", key)
	}

	var values []*float64
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode %s: %w", key, err)
	}

	return values, nil
}

func nullableValueAt(values []*float64, index int) *float64 {
	if index < 0 || index >= len(values) {
		return nil
	}

	return values[index]
}

func decodeMemberSeries(
	daily map[string]json.RawMessage,
	prefix string,
) ([][]*float64, error) {
	keys := make([]string, 0)
	for key := range daily {
		if key == prefix || strings.HasPrefix(key, prefix+"_member") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		return nil, fmt.Errorf("missing ensemble field %s", prefix)
	}

	series := make([][]*float64, 0, len(keys))
	for _, key := range keys {
		values, err := decodeNullableFloatSeries(daily, key)
		if err != nil {
			return nil, err
		}
		series = append(series, values)
	}

	return series, nil
}

func memberValuesAt(series [][]*float64, index int) []float64 {
	values := make([]float64, 0, len(series))
	for _, member := range series {
		value := nullableValueAt(member, index)
		if value != nil {
			values = append(values, *value)
		}
	}

	return values
}

func quantile(values []float64, probability float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	position := math.Max(0, math.Min(1, probability)) * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}

	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
