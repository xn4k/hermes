package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
)

const (
	openMeteoForecastURL = "https://api.open-meteo.com/v1/forecast"
	openMeteoGeocodeURL  = "https://geocoding-api.open-meteo.com/v1/search"
)

var defaultWeatherLocation = WeatherLocation{
	Name:      "Köln-Dünnwald (51069)",
	Admin1:    "Nordrhein-Westfalen",
	Country:   "Deutschland",
	Latitude:  51.00384,
	Longitude: 7.04576,
	Timezone:  "Europe/Berlin",
}

type WeatherLocation struct {
	Name      string  `json:"name"`
	Admin1    string  `json:"admin1"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

type WeatherCurrent struct {
	Time                string  `json:"time"`
	Temperature         float64 `json:"temperature"`
	ApparentTemperature float64 `json:"apparentTemperature"`
	WeatherCode         int     `json:"weatherCode"`
	WindSpeed           float64 `json:"windSpeed"`
}

type WeatherHourlyPoint struct {
	Time                     string  `json:"time"`
	Temperature              float64 `json:"temperature"`
	PrecipitationProbability float64 `json:"precipitationProbability"`
	Precipitation            float64 `json:"precipitation"`
	WeatherCode              int     `json:"weatherCode"`
}

type WeatherDailyPoint struct {
	Date                     string  `json:"date"`
	TemperatureMin           float64 `json:"temperatureMin"`
	TemperatureMax           float64 `json:"temperatureMax"`
	ApparentTemperatureMax   float64 `json:"apparentTemperatureMax"`
	PrecipitationProbability float64 `json:"precipitationProbability"`
	WeatherCode              int     `json:"weatherCode"`
	Sunrise                  string  `json:"sunrise"`
	Sunset                   string  `json:"sunset"`
}

type WeatherModelForecast struct {
	ID     string               `json:"id"`
	Name   string               `json:"name"`
	Short  string               `json:"short"`
	Color  string               `json:"color"`
	Hourly []WeatherHourlyPoint `json:"hourly"`
	Daily  []WeatherDailyPoint  `json:"daily"`
}

type WeatherConsensus struct {
	TodayMaxMedian       float64 `json:"todayMaxMedian"`
	TodayMaxMin          float64 `json:"todayMaxMin"`
	TodayMaxMax          float64 `json:"todayMaxMax"`
	TemperatureSpread    float64 `json:"temperatureSpread"`
	Confidence           string  `json:"confidence"`
	RainAgreementNext6   int     `json:"rainAgreementNext6"`
	RainProbabilityNext6 float64 `json:"rainProbabilityNext6"`
	RainStart            string  `json:"rainStart"`
	HeatLevel            string  `json:"heatLevel"`
	HeatMessage          string  `json:"heatMessage"`
}

type WeatherForecast struct {
	Location    WeatherLocation        `json:"location"`
	Current     WeatherCurrent         `json:"current"`
	Models      []WeatherModelForecast `json:"models"`
	Consensus   WeatherConsensus       `json:"consensus"`
	RefreshedAt time.Time              `json:"refreshedAt"`
	Source      string                 `json:"source"`
}

type weatherCacheEntry struct {
	forecast  WeatherForecast
	expiresAt time.Time
}

type WeatherService struct {
	client   *http.Client
	cacheTTL time.Duration
	mu       sync.RWMutex
	cache    map[string]weatherCacheEntry
}

type updateWeatherLocationRequest struct {
	Name      string  `json:"name"`
	Admin1    string  `json:"admin1"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

type openMeteoResponse struct {
	Current struct {
		Time                string  `json:"time"`
		Temperature         float64 `json:"temperature_2m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
		WeatherCode         int     `json:"weather_code"`
		WindSpeed           float64 `json:"wind_speed_10m"`
	} `json:"current"`
	Hourly struct {
		Time []string `json:"time"`

		TemperatureICON []float64 `json:"temperature_2m_icon_seamless"`
		RainChanceICON  []float64 `json:"precipitation_probability_icon_seamless"`
		PrecipICON      []float64 `json:"precipitation_icon_seamless"`
		CodeICON        []float64 `json:"weather_code_icon_seamless"`

		TemperatureECMWF []float64 `json:"temperature_2m_ecmwf_ifs025"`
		RainChanceECMWF  []float64 `json:"precipitation_probability_ecmwf_ifs025"`
		PrecipECMWF      []float64 `json:"precipitation_ecmwf_ifs025"`
		CodeECMWF        []float64 `json:"weather_code_ecmwf_ifs025"`

		TemperatureGFS []float64 `json:"temperature_2m_gfs_seamless"`
		RainChanceGFS  []float64 `json:"precipitation_probability_gfs_seamless"`
		PrecipGFS      []float64 `json:"precipitation_gfs_seamless"`
		CodeGFS        []float64 `json:"weather_code_gfs_seamless"`
	} `json:"hourly"`
	Daily struct {
		Time []string `json:"time"`

		TemperatureMaxICON []float64 `json:"temperature_2m_max_icon_seamless"`
		TemperatureMinICON []float64 `json:"temperature_2m_min_icon_seamless"`
		ApparentMaxICON    []float64 `json:"apparent_temperature_max_icon_seamless"`
		RainChanceICON     []float64 `json:"precipitation_probability_max_icon_seamless"`
		CodeICON           []float64 `json:"weather_code_icon_seamless"`
		SunriseICON        []string  `json:"sunrise_icon_seamless"`
		SunsetICON         []string  `json:"sunset_icon_seamless"`

		TemperatureMaxECMWF []float64 `json:"temperature_2m_max_ecmwf_ifs025"`
		TemperatureMinECMWF []float64 `json:"temperature_2m_min_ecmwf_ifs025"`
		ApparentMaxECMWF    []float64 `json:"apparent_temperature_max_ecmwf_ifs025"`
		RainChanceECMWF     []float64 `json:"precipitation_probability_max_ecmwf_ifs025"`
		CodeECMWF           []float64 `json:"weather_code_ecmwf_ifs025"`
		SunriseECMWF        []string  `json:"sunrise_ecmwf_ifs025"`
		SunsetECMWF         []string  `json:"sunset_ecmwf_ifs025"`

		TemperatureMaxGFS []float64 `json:"temperature_2m_max_gfs_seamless"`
		TemperatureMinGFS []float64 `json:"temperature_2m_min_gfs_seamless"`
		ApparentMaxGFS    []float64 `json:"apparent_temperature_max_gfs_seamless"`
		RainChanceGFS     []float64 `json:"precipitation_probability_max_gfs_seamless"`
		CodeGFS           []float64 `json:"weather_code_gfs_seamless"`
		SunriseGFS        []string  `json:"sunrise_gfs_seamless"`
		SunsetGFS         []string  `json:"sunset_gfs_seamless"`
	} `json:"daily"`
}

type geocodingResponse struct {
	Results []struct {
		Name        string  `json:"name"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Timezone    string  `json:"timezone"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		Admin1      string  `json:"admin1"`
	} `json:"results"`
}

func NewWeatherService() *WeatherService {
	return &WeatherService{
		client: &http.Client{
			Timeout: 12 * time.Second,
		},
		cacheTTL: 15 * time.Minute,
		cache:    make(map[string]weatherCacheEntry),
	}
}

func migrateWeather(ctx context.Context, app *App) error {
	_, err := app.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS user_weather_settings (
			user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			location_name TEXT NOT NULL,
			admin1 TEXT NOT NULL DEFAULT '',
			country TEXT NOT NULL DEFAULT '',
			latitude DOUBLE PRECISION NOT NULL,
			longitude DOUBLE PRECISION NOT NULL,
			timezone TEXT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)

	return err
}

func (app *App) weatherLocation(ctx context.Context, userID int64) (WeatherLocation, error) {
	var location WeatherLocation

	err := app.db.QueryRow(
		ctx,
		`
		SELECT location_name, admin1, country, latitude, longitude, timezone
		FROM user_weather_settings
		WHERE user_id = $1
		`,
		userID,
	).Scan(
		&location.Name,
		&location.Admin1,
		&location.Country,
		&location.Latitude,
		&location.Longitude,
		&location.Timezone,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return defaultWeatherLocation, nil
	}

	return location, err
}

func (app *App) handleGetWeather(c *echo.Context) error {
	if view := strings.TrimSpace(c.QueryParam("view")); view != "" {
		return app.handleWeatherOutlook(c, view, false)
	}

	user, err := app.currentUser(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "not_authenticated"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 14*time.Second)
	defer cancel()

	location, err := app.weatherLocation(ctx, user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "weather_settings_failed"})
	}

	forecast, err := app.weather.forecast(ctx, location, false)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{
			"error":   "weather_fetch_failed",
			"details": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, forecast)
}

func (app *App) handleRefreshWeather(c *echo.Context) error {
	if view := strings.TrimSpace(c.QueryParam("view")); view != "" {
		return app.handleWeatherOutlook(c, view, true)
	}

	user, err := app.currentUser(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "not_authenticated"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 14*time.Second)
	defer cancel()

	location, err := app.weatherLocation(ctx, user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "weather_settings_failed"})
	}

	forecast, err := app.weather.forecast(ctx, location, true)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{
			"error":   "weather_fetch_failed",
			"details": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, forecast)
}

func (app *App) handleSearchWeatherLocations(c *echo.Context) error {
	if _, err := app.currentUser(c); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "not_authenticated"})
	}

	query := strings.TrimSpace(c.QueryParam("q"))
	if len(query) < 2 || len(query) > 120 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid_location_query"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 8*time.Second)
	defer cancel()

	locations, err := app.weather.searchLocations(ctx, query)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{
			"error":   "location_search_failed",
			"details": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{"locations": locations})
}

func (app *App) handleUpdateWeatherLocation(c *echo.Context) error {
	user, err := app.currentUser(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "not_authenticated"})
	}

	var req updateWeatherLocationRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid_json"})
	}

	location := WeatherLocation{
		Name:      strings.TrimSpace(req.Name),
		Admin1:    strings.TrimSpace(req.Admin1),
		Country:   strings.TrimSpace(req.Country),
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		Timezone:  strings.TrimSpace(req.Timezone),
	}

	if location.Name == "" ||
		len(location.Name) > 160 ||
		location.Latitude < -90 ||
		location.Latitude > 90 ||
		location.Longitude < -180 ||
		location.Longitude > 180 ||
		location.Timezone == "" ||
		len(location.Timezone) > 80 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid_location"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	_, err = app.db.Exec(
		ctx,
		`
		INSERT INTO user_weather_settings (
			user_id, location_name, admin1, country, latitude, longitude, timezone
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			location_name = EXCLUDED.location_name,
			admin1 = EXCLUDED.admin1,
			country = EXCLUDED.country,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			timezone = EXCLUDED.timezone,
			updated_at = NOW()
		`,
		user.ID,
		location.Name,
		location.Admin1,
		location.Country,
		location.Latitude,
		location.Longitude,
		location.Timezone,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "weather_settings_failed"})
	}

	return c.JSON(http.StatusOK, map[string]any{"location": location})
}

func (service *WeatherService) searchLocations(ctx context.Context, query string) ([]WeatherLocation, error) {
	params := url.Values{}
	params.Set("name", query)
	params.Set("count", "8")
	params.Set("language", "de")
	params.Set("format", "json")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, openMeteoGeocodeURL+"?"+params.Encode(), nil)
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
		return nil, fmt.Errorf("geocoding returned HTTP %d", response.StatusCode)
	}

	var payload geocodingResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}

	locations := make([]WeatherLocation, 0, len(payload.Results))
	for _, result := range payload.Results {
		timezone := result.Timezone
		if timezone == "" {
			timezone = "auto"
		}

		locations = append(locations, WeatherLocation{
			Name:      result.Name,
			Admin1:    result.Admin1,
			Country:   result.Country,
			Latitude:  result.Latitude,
			Longitude: result.Longitude,
			Timezone:  timezone,
		})
	}

	return locations, nil
}

func (service *WeatherService) forecast(
	ctx context.Context,
	location WeatherLocation,
	force bool,
) (WeatherForecast, error) {
	key := weatherCacheKey(location)

	if !force {
		if cached, ok := service.cachedForecast(key, false); ok {
			cached.Source = "cache"
			return cached, nil
		}
	}

	forecast, err := service.fetchForecast(ctx, location)
	if err != nil {
		if cached, ok := service.cachedForecast(key, true); ok {
			cached.Source = "stale"
			return cached, nil
		}

		return WeatherForecast{}, err
	}

	service.mu.Lock()
	service.cache[key] = weatherCacheEntry{
		forecast:  forecast,
		expiresAt: time.Now().Add(service.cacheTTL),
	}
	service.mu.Unlock()

	return forecast, nil
}

func (service *WeatherService) cachedForecast(key string, allowExpired bool) (WeatherForecast, bool) {
	service.mu.RLock()
	entry, ok := service.cache[key]
	service.mu.RUnlock()

	if !ok || (!allowExpired && time.Now().After(entry.expiresAt)) {
		return WeatherForecast{}, false
	}

	return entry.forecast, true
}

func weatherCacheKey(location WeatherLocation) string {
	return fmt.Sprintf(
		"%.4f:%.4f:%s",
		location.Latitude,
		location.Longitude,
		location.Timezone,
	)
}

func (service *WeatherService) fetchForecast(
	ctx context.Context,
	location WeatherLocation,
) (WeatherForecast, error) {
	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(location.Latitude, 'f', 5, 64))
	params.Set("longitude", strconv.FormatFloat(location.Longitude, 'f', 5, 64))
	params.Set("models", "icon_seamless,ecmwf_ifs025,gfs_seamless")
	params.Set("current", "temperature_2m,apparent_temperature,weather_code,wind_speed_10m")
	params.Set("hourly", "temperature_2m,precipitation_probability,precipitation,weather_code")
	params.Set(
		"daily",
		"temperature_2m_max,temperature_2m_min,apparent_temperature_max,"+
			"precipitation_probability_max,weather_code,sunrise,sunset",
	)
	params.Set("timezone", location.Timezone)
	params.Set("forecast_days", "5")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, openMeteoForecastURL+"?"+params.Encode(), nil)
	if err != nil {
		return WeatherForecast{}, err
	}
	request.Header.Set("User-Agent", "Hermes Dashboard/1.0")

	response, err := service.client.Do(request)
	if err != nil {
		return WeatherForecast{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return WeatherForecast{}, fmt.Errorf("forecast returned HTTP %d", response.StatusCode)
	}

	var payload openMeteoResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return WeatherForecast{}, err
	}

	models := []WeatherModelForecast{
		normalizeWeatherModel(
			"icon",
			"DWD ICON",
			"ICON",
			"#5ee6a8",
			payload.Hourly.Time,
			payload.Hourly.TemperatureICON,
			payload.Hourly.RainChanceICON,
			payload.Hourly.PrecipICON,
			payload.Hourly.CodeICON,
			payload.Daily.Time,
			payload.Daily.TemperatureMinICON,
			payload.Daily.TemperatureMaxICON,
			payload.Daily.ApparentMaxICON,
			payload.Daily.RainChanceICON,
			payload.Daily.CodeICON,
			payload.Daily.SunriseICON,
			payload.Daily.SunsetICON,
		),
		normalizeWeatherModel(
			"ecmwf",
			"ECMWF IFS",
			"IFS",
			"#7aa2ff",
			payload.Hourly.Time,
			payload.Hourly.TemperatureECMWF,
			payload.Hourly.RainChanceECMWF,
			payload.Hourly.PrecipECMWF,
			payload.Hourly.CodeECMWF,
			payload.Daily.Time,
			payload.Daily.TemperatureMinECMWF,
			payload.Daily.TemperatureMaxECMWF,
			payload.Daily.ApparentMaxECMWF,
			payload.Daily.RainChanceECMWF,
			payload.Daily.CodeECMWF,
			payload.Daily.SunriseECMWF,
			payload.Daily.SunsetECMWF,
		),
		normalizeWeatherModel(
			"gfs",
			"NOAA GFS",
			"GFS",
			"#ffbd6e",
			payload.Hourly.Time,
			payload.Hourly.TemperatureGFS,
			payload.Hourly.RainChanceGFS,
			payload.Hourly.PrecipGFS,
			payload.Hourly.CodeGFS,
			payload.Daily.Time,
			payload.Daily.TemperatureMinGFS,
			payload.Daily.TemperatureMaxGFS,
			payload.Daily.ApparentMaxGFS,
			payload.Daily.RainChanceGFS,
			payload.Daily.CodeGFS,
			payload.Daily.SunriseGFS,
			payload.Daily.SunsetGFS,
		),
	}

	for _, model := range models {
		if len(model.Hourly) == 0 || len(model.Daily) == 0 {
			return WeatherForecast{}, fmt.Errorf("model %s returned incomplete data", model.ID)
		}
	}

	forecast := WeatherForecast{
		Location: location,
		Current: WeatherCurrent{
			Time:                payload.Current.Time,
			Temperature:         payload.Current.Temperature,
			ApparentTemperature: payload.Current.ApparentTemperature,
			WeatherCode:         payload.Current.WeatherCode,
			WindSpeed:           payload.Current.WindSpeed,
		},
		Models:      models,
		RefreshedAt: time.Now(),
		Source:      "refresh",
	}
	forecast.Consensus = buildWeatherConsensus(forecast)

	return forecast, nil
}

func normalizeWeatherModel(
	id string,
	name string,
	short string,
	color string,
	hourlyTimes []string,
	temperatures []float64,
	rainChances []float64,
	precipitation []float64,
	weatherCodes []float64,
	dailyTimes []string,
	minTemperatures []float64,
	maxTemperatures []float64,
	apparentMax []float64,
	dailyRainChances []float64,
	dailyWeatherCodes []float64,
	sunrises []string,
	sunsets []string,
) WeatherModelForecast {
	hourlyLength := minimumLength(
		len(hourlyTimes),
		len(temperatures),
		len(rainChances),
		len(precipitation),
		len(weatherCodes),
	)
	hourly := make([]WeatherHourlyPoint, 0, hourlyLength)
	for index := 0; index < hourlyLength; index++ {
		hourly = append(hourly, WeatherHourlyPoint{
			Time:                     hourlyTimes[index],
			Temperature:              temperatures[index],
			PrecipitationProbability: rainChances[index],
			Precipitation:            precipitation[index],
			WeatherCode:              int(weatherCodes[index]),
		})
	}

	dailyLength := minimumLength(
		len(dailyTimes),
		len(minTemperatures),
		len(maxTemperatures),
		len(apparentMax),
		len(dailyRainChances),
		len(dailyWeatherCodes),
		len(sunrises),
		len(sunsets),
	)
	daily := make([]WeatherDailyPoint, 0, dailyLength)
	for index := 0; index < dailyLength; index++ {
		daily = append(daily, WeatherDailyPoint{
			Date:                     dailyTimes[index],
			TemperatureMin:           minTemperatures[index],
			TemperatureMax:           maxTemperatures[index],
			ApparentTemperatureMax:   apparentMax[index],
			PrecipitationProbability: dailyRainChances[index],
			WeatherCode:              int(dailyWeatherCodes[index]),
			Sunrise:                  sunrises[index],
			Sunset:                   sunsets[index],
		})
	}

	return WeatherModelForecast{
		ID:     id,
		Name:   name,
		Short:  short,
		Color:  color,
		Hourly: hourly,
		Daily:  daily,
	}
}

func minimumLength(lengths ...int) int {
	if len(lengths) == 0 {
		return 0
	}

	minimum := lengths[0]
	for _, length := range lengths[1:] {
		if length < minimum {
			minimum = length
		}
	}

	return minimum
}

func buildWeatherConsensus(forecast WeatherForecast) WeatherConsensus {
	consensus := WeatherConsensus{
		Confidence: "low",
		HeatLevel:  "normal",
	}

	var todayMaximums []float64
	for _, model := range forecast.Models {
		if len(model.Daily) > 0 {
			todayMaximums = append(todayMaximums, model.Daily[0].TemperatureMax)
		}
	}

	if len(todayMaximums) > 0 {
		consensus.TodayMaxMedian = median(todayMaximums)
		consensus.TodayMaxMin = minimumFloat(todayMaximums)
		consensus.TodayMaxMax = maximumFloat(todayMaximums)
		consensus.TemperatureSpread = consensus.TodayMaxMax - consensus.TodayMaxMin

		switch {
		case consensus.TemperatureSpread <= 2:
			consensus.Confidence = "high"
		case consensus.TemperatureSpread <= 4:
			consensus.Confidence = "medium"
		default:
			consensus.Confidence = "low"
		}
	}

	buildRainConsensus(forecast, &consensus)
	buildHeatConsensus(forecast, &consensus)

	return consensus
}

func buildRainConsensus(forecast WeatherForecast, consensus *WeatherConsensus) {
	if len(forecast.Models) == 0 {
		return
	}

	startIndex := 0
	for index, point := range forecast.Models[0].Hourly {
		if point.Time >= forecast.Current.Time {
			startIndex = index
			break
		}
	}

	endIndex := startIndex + 6
	if endIndex > len(forecast.Models[0].Hourly) {
		endIndex = len(forecast.Models[0].Hourly)
	}

	bestMedian := 0.0
	bestAgreement := 0

	for index := startIndex; index < endIndex; index++ {
		var probabilities []float64
		agreement := 0

		for _, model := range forecast.Models {
			if index >= len(model.Hourly) {
				continue
			}

			probability := model.Hourly[index].PrecipitationProbability
			probabilities = append(probabilities, probability)
			if probability >= 40 || model.Hourly[index].Precipitation >= 0.2 {
				agreement++
			}
		}

		value := median(probabilities)
		if value > bestMedian {
			bestMedian = value
			bestAgreement = agreement
		}

		if consensus.RainStart == "" && value >= 40 {
			consensus.RainStart = forecast.Models[0].Hourly[index].Time
		}
	}

	consensus.RainProbabilityNext6 = bestMedian
	consensus.RainAgreementNext6 = bestAgreement
}

func buildHeatConsensus(forecast WeatherForecast, consensus *WeatherConsensus) {
	if len(forecast.Models) == 0 {
		return
	}

	days := len(forecast.Models[0].Daily)
	hotDays := 0
	highestApparent := 0.0

	for day := 0; day < days; day++ {
		var apparentValues []float64
		for _, model := range forecast.Models {
			if day < len(model.Daily) {
				apparentValues = append(
					apparentValues,
					model.Daily[day].ApparentTemperatureMax,
				)
			}
		}

		value := median(apparentValues)
		if value >= 30 {
			hotDays++
		}
		if value > highestApparent {
			highestApparent = value
		}
	}

	switch {
	case highestApparent >= 35:
		consensus.HeatLevel = "danger"
		consensus.HeatMessage = "Sehr hohe Wärmebelastung im Modellkonsens."
	case hotDays >= 2:
		consensus.HeatLevel = "warning"
		consensus.HeatMessage = fmt.Sprintf("%d heiße Tage im Modellkonsens.", hotDays)
	case highestApparent >= 30:
		consensus.HeatLevel = "notice"
		consensus.HeatMessage = "Ein heißer Tag zeichnet sich ab."
	}
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}

	return sorted[middle]
}

func minimumFloat(values []float64) float64 {
	minimum := math.Inf(1)
	for _, value := range values {
		if value < minimum {
			minimum = value
		}
	}

	if math.IsInf(minimum, 1) {
		return 0
	}

	return minimum
}

func maximumFloat(values []float64) float64 {
	maximum := math.Inf(-1)
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}

	if math.IsInf(maximum, -1) {
		return 0
	}

	return maximum
}
