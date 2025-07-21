package chat

import (
	"context"
	"encoding/json"
	"fmt"

	llm_memory "xiaozhi-esp32-server-golang/internal/domain/llm/memory"
	"xiaozhi-esp32-server-golang/internal/util"
	log "xiaozhi-esp32-server-golang/logger"
)

//此文件处理 local mcp tool 与 session绑定 的工具调用

// 关闭会话
func (c *ChatManager) LocalMcpCloseChat() error {
	c.Close()
	return nil
}

// 清空历史对话
func (c *ChatManager) LocalMcpClearHistory() error {
	llm_memory.Get().ResetMemory(c.ctx, c.DeviceID)
	return nil
}

// LocalMcpGetWeather 获取当前天气
func (c *ChatManager) LocalMcpGetWeather(ctx context.Context, city string) (string, error) {
	log.Info("执行天气查询工具")

	// 如果没有提供城市，使用用户当前位置
	if city == "" {
		clientState := c.GetClientState()
		if clientState.LocationInfo != nil && clientState.LocationInfo.City != "" && clientState.LocationInfo.City != "未知" {
			city = clientState.LocationInfo.City
			log.Infof("使用用户当前位置查询天气: %s", city)
		}
	}

	if city == "" {
		return `{"success": false, "error": "未指定城市名称，且无法获取用户位置信息"}`, nil
	}

	// 调用高德API获取天气
	amapAPI := util.NewAmapAPI()
	weather, err := amapAPI.GetCurrentWeather(ctx, city)
	if err != nil {
		log.Errorf("获取天气信息失败: %v", err)
		return fmt.Sprintf(`{"success": false, "error": "获取天气信息失败: %s"}`, err.Error()), nil
	}

	// 构造返回结果
	result := map[string]interface{}{
		"success":     true,
		"city":        weather.City,
		"province":    weather.Province,
		"weather":     weather.Weather,
		"temperature": weather.Temperature,
		"humidity":    weather.Humidity,
		"wind": map[string]string{
			"direction": weather.Winddirection,
			"power":     weather.Windpower,
		},
		"report_time": weather.ReportTime,
		"description": fmt.Sprintf("%s%s当前天气：%s，温度%s°C，湿度%s%%，%s风%s级",
			weather.Province, weather.City, weather.Weather, weather.Temperature,
			weather.Humidity, weather.Winddirection, weather.Windpower),
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return `{"success": false, "error": "序列化结果失败"}`, err
	}

	log.Infof("天气查询成功: %s - %s", city, weather.Weather)
	return string(resultBytes), nil
}

// LocalMcpGetWeatherForecast 获取天气预报
func (c *ChatManager) LocalMcpGetWeatherForecast(ctx context.Context, city string) (string, error) {
	log.Info("执行天气预报查询工具")

	// 如果没有提供城市，使用用户当前位置
	if city == "" {
		clientState := c.GetClientState()
		if clientState.LocationInfo != nil && clientState.LocationInfo.City != "" && clientState.LocationInfo.City != "未知" {
			city = clientState.LocationInfo.City
			log.Infof("使用用户当前位置查询天气预报: %s", city)
		}
	}

	if city == "" {
		return `{"success": false, "error": "未指定城市名称，且无法获取用户位置信息"}`, nil
	}

	// 调用高德API获取天气预报
	amapAPI := util.NewAmapAPI()
	forecast, err := amapAPI.GetWeatherForecast(ctx, city)
	if err != nil {
		log.Errorf("获取天气预报失败: %v", err)
		return fmt.Sprintf(`{"success": false, "error": "获取天气预报失败: %s"}`, err.Error()), nil
	}

	// 构造返回结果
	forecastDays := make([]map[string]interface{}, 0)
	description := fmt.Sprintf("%s%s未来几天天气预报：\n", forecast.Province, forecast.City)

	for _, cast := range forecast.Casts {
		dayInfo := map[string]interface{}{
			"date":          cast.Date,
			"week":          cast.Week,
			"day_weather":   cast.DayWeather,
			"night_weather": cast.NightWeather,
			"day_temp":      cast.DayTemp,
			"night_temp":    cast.NightTemp,
			"day_wind":      cast.DayWind + cast.DayPower + "级",
			"night_wind":    cast.NightWind + cast.NightPower + "级",
		}
		forecastDays = append(forecastDays, dayInfo)

		description += fmt.Sprintf("%s（%s）：白天%s %s°C，夜间%s %s°C，%s\n",
			cast.Date, cast.Week, cast.DayWeather, cast.DayTemp,
			cast.NightWeather, cast.NightTemp, cast.DayWind+cast.DayPower+"级")
	}

	result := map[string]interface{}{
		"success":     true,
		"city":        forecast.City,
		"province":    forecast.Province,
		"report_time": forecast.ReportTime,
		"forecasts":   forecastDays,
		"description": description,
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return `{"success": false, "error": "序列化结果失败"}`, err
	}

	log.Infof("天气预报查询成功: %s", city)
	return string(resultBytes), nil
}
