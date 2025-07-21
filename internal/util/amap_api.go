package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "xiaozhi-esp32-server-golang/logger"

	"github.com/spf13/viper"
)

// AmapIPLocationResponse 高德IP定位响应
type AmapIPLocationResponse struct {
	Status    string `json:"status"`
	Info      string `json:"info"`
	Infocode  string `json:"infocode"`
	Province  string `json:"province"`
	City      string `json:"city"`
	Adcode    string `json:"adcode"`
	Rectangle string `json:"rectangle"`
}

// AmapWeatherResponse 高德天气查询响应
type AmapWeatherResponse struct {
	Status    string                `json:"status"`
	Count     string                `json:"count"`
	Info      string                `json:"info"`
	Infocode  string                `json:"infocode"`
	Lives     []AmapWeatherLive     `json:"lives,omitempty"`
	Forecasts []AmapWeatherForecast `json:"forecasts,omitempty"`
}

// AmapWeatherLive 实时天气
type AmapWeatherLive struct {
	Province         string `json:"province"`
	City             string `json:"city"`
	Adcode           string `json:"adcode"`
	Weather          string `json:"weather"`
	Temperature      string `json:"temperature"`
	Winddirection    string `json:"winddirection"`
	Windpower        string `json:"windpower"`
	Humidity         string `json:"humidity"`
	ReportTime       string `json:"reporttime"`
	TemperatureFloat string `json:"temperature_float"`
	HumidityFloat    string `json:"humidity_float"`
}

// AmapWeatherForecast 天气预报
type AmapWeatherForecast struct {
	City       string            `json:"city"`
	Adcode     string            `json:"adcode"`
	Province   string            `json:"province"`
	ReportTime string            `json:"reporttime"`
	Casts      []AmapWeatherCast `json:"casts"`
}

// AmapWeatherCast 天气预报详情
type AmapWeatherCast struct {
	Date           string `json:"date"`
	Week           string `json:"week"`
	DayWeather     string `json:"dayweather"`
	NightWeather   string `json:"nightweather"`
	DayTemp        string `json:"daytemp"`
	NightTemp      string `json:"nighttemp"`
	DayWind        string `json:"daywind"`
	NightWind      string `json:"nightwind"`
	DayPower       string `json:"daypower"`
	NightPower     string `json:"nightpower"`
	DayTempFloat   string `json:"daytemp_float"`
	NightTempFloat string `json:"nighttemp_float"`
}

// LocationInfo 位置信息
type LocationInfo struct {
	IP       string  `json:"ip"`
	Province string  `json:"province"`
	City     string  `json:"city"`
	District string  `json:"district"`
	Address  string  `json:"address"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	CityCode string  `json:"city_code"`
}

// AmapAPI 高德API客户端
type AmapAPI struct {
	apiKey string
	client *http.Client
}

// NewAmapAPI 创建高德API客户端
func NewAmapAPI() *AmapAPI {
	apiKey := viper.GetString("amap.api_key")
	if apiKey == "" {
		log.Warn("高德API Key未配置")
	}

	return &AmapAPI{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetLocationByIP 根据IP获取位置信息
func (a *AmapAPI) GetLocationByIP(ctx context.Context, ip string) (*LocationInfo, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("高德API Key未配置")
	}

	// 过滤本地IP
	if ip == "" || ip == "unknown" || strings.HasPrefix(ip, "127.") ||
		strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") ||
		strings.Contains(ip, ":") {
		log.Debugf("跳过本地IP定位: %s", ip)
		return &LocationInfo{
			IP:       ip,
			Province: "未知",
			City:     "本地",
			District: "",
			Address:  "本地网络",
		}, nil
	}

	apiUrl := viper.GetString("amap.ip_location_url")
	if apiUrl == "" {
		apiUrl = "https://restapi.amap.com/v3/ip"
	}

	params := url.Values{}
	params.Set("key", a.apiKey)
	params.Set("ip", ip)

	reqUrl := fmt.Sprintf("%s?%s", apiUrl, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result AmapIPLocationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if result.Status != "1" {
		return nil, fmt.Errorf("高德API错误: %s", result.Info)
	}

	location := &LocationInfo{
		IP:       ip,
		Province: result.Province,
		City:     result.City,
		District: "",
		Address:  fmt.Sprintf("%s%s", result.Province, result.City),
		CityCode: result.Adcode,
	}

	log.Debugf("IP位置查询成功: %s -> %s", ip, location.Address)
	return location, nil
}

// GetWeatherByCity 根据城市获取天气信息
func (a *AmapAPI) GetWeatherByCity(ctx context.Context, city string, extensions string) (*AmapWeatherResponse, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("高德API Key未配置")
	}

	apiUrl := viper.GetString("amap.weather_url")
	if apiUrl == "" {
		apiUrl = "https://restapi.amap.com/v3/weather/weatherInfo"
	}

	params := url.Values{}
	params.Set("key", a.apiKey)
	params.Set("city", city)
	if extensions == "" {
		extensions = "base" // base=实时天气, all=预报天气
	}
	params.Set("extensions", extensions)

	reqUrl := fmt.Sprintf("%s?%s", apiUrl, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result AmapWeatherResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if result.Status != "1" {
		return nil, fmt.Errorf("高德API错误: %s", result.Info)
	}

	return &result, nil
}

// GetCurrentWeather 获取实时天气
func (a *AmapAPI) GetCurrentWeather(ctx context.Context, city string) (*AmapWeatherLive, error) {
	weather, err := a.GetWeatherByCity(ctx, city, "base")
	if err != nil {
		return nil, err
	}

	if len(weather.Lives) == 0 {
		return nil, fmt.Errorf("未找到天气数据")
	}

	return &weather.Lives[0], nil
}

// GetWeatherForecast 获取天气预报
func (a *AmapAPI) GetWeatherForecast(ctx context.Context, city string) (*AmapWeatherForecast, error) {
	weather, err := a.GetWeatherByCity(ctx, city, "all")
	if err != nil {
		return nil, err
	}

	if len(weather.Forecasts) == 0 {
		return nil, fmt.Errorf("未找到天气预报数据")
	}

	return &weather.Forecasts[0], nil
}
