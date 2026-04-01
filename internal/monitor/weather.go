package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// WeatherData is the cached weather info pushed via WebSocket.
type WeatherData struct {
	Temperature float64 `json:"temperature"`
	WeatherCode int     `json:"weather_code"`
	WindSpeed   float64 `json:"wind_speed"`
	City        string  `json:"city"`
	Country     string  `json:"country"`
	Icon        string  `json:"icon"`
	Description string  `json:"description"`
}

var (
	weatherMu    sync.RWMutex
	weatherCache *WeatherData
	weatherLat   float64
	weatherLon   float64
	weatherCity  string
	weatherCountry string
	locationDone bool
)

// GetWeather returns the cached weather data.
func GetWeather() *WeatherData {
	weatherMu.RLock()
	defer weatherMu.RUnlock()
	return weatherCache
}

// StartWeatherLoop detects location once, then fetches weather every 30 minutes.
func StartWeatherLoop() {
	go func() {
		detectLocation()
		fetchWeather()
		ticker := time.NewTicker(30 * time.Minute)
		for range ticker.C {
			fetchWeather()
		}
	}()
}

// detectLocation uses IP geolocation to find lat/lon/city.
func detectLocation() {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/?fields=city,country,lat,lon")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var result struct {
		City    string  `json:"city"`
		Country string  `json:"country"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) == nil && result.Lat != 0 {
		weatherMu.Lock()
		weatherLat = result.Lat
		weatherLon = result.Lon
		weatherCity = result.City
		weatherCountry = result.Country
		locationDone = true
		weatherMu.Unlock()
	}
}

// fetchWeather calls Open-Meteo API for current weather.
func fetchWeather() {
	weatherMu.RLock()
	if !locationDone {
		weatherMu.RUnlock()
		return
	}
	lat := weatherLat
	lon := weatherLon
	city := weatherCity
	country := weatherCountry
	weatherMu.RUnlock()

	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,weather_code,wind_speed_10m&timezone=auto",
		lat, lon,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var result struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
			WeatherCode int     `json:"weather_code"`
			WindSpeed   float64 `json:"wind_speed_10m"`
		} `json:"current"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return
	}

	icon, desc := weatherCodeToIconDesc(result.Current.WeatherCode)

	weatherMu.Lock()
	weatherCache = &WeatherData{
		Temperature: result.Current.Temperature,
		WeatherCode: result.Current.WeatherCode,
		WindSpeed:   result.Current.WindSpeed,
		City:        city,
		Country:     country,
		Icon:        icon,
		Description: desc,
	}
	weatherMu.Unlock()
}

// weatherCodeToIconDesc maps WMO weather codes to icon names and descriptions.
func weatherCodeToIconDesc(code int) (string, string) {
	switch {
	case code == 0:
		return "clear_day", "Clear"
	case code == 1:
		return "partly_cloudy_day", "Mostly Clear"
	case code == 2:
		return "partly_cloudy_day", "Partly Cloudy"
	case code == 3:
		return "cloud", "Overcast"
	case code >= 45 && code <= 48:
		return "foggy", "Fog"
	case code >= 51 && code <= 55:
		return "rainy", "Drizzle"
	case code >= 56 && code <= 57:
		return "weather_snowy", "Freezing Drizzle"
	case code >= 61 && code <= 65:
		return "rainy", "Rain"
	case code >= 66 && code <= 67:
		return "weather_snowy", "Freezing Rain"
	case code >= 71 && code <= 77:
		return "weather_snowy", "Snow"
	case code >= 80 && code <= 82:
		return "rainy", "Rain Showers"
	case code >= 85 && code <= 86:
		return "weather_snowy", "Snow Showers"
	case code == 95:
		return "thunderstorm", "Thunderstorm"
	case code >= 96 && code <= 99:
		return "thunderstorm", "Thunderstorm with Hail"
	default:
		return "partly_cloudy_day", "Unknown"
	}
}
