package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"xiaozhi-esp32-server-golang/internal/data/client"
	user_config "xiaozhi-esp32-server-golang/internal/domain/config"
	ctypes "xiaozhi-esp32-server-golang/internal/domain/config/types"
	"xiaozhi-esp32-server-golang/internal/util"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/spf13/viper"
)

type ActivationRequest struct {
	Payload ctypes.ActivationPayload `json:"Payload"`
}

func (s *WebSocketServer) handleOta(w http.ResponseWriter, r *http.Request) {
	//获取客户端ip
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}

	//从header头部获取Device-Id和Client-Id
	deviceId := r.Header.Get("Device-Id")
	clientId := r.Header.Get("Client-Id")

	if deviceId == "" || clientId == "" {
		log.Errorf("缺少Device-Id或Client-Id")
		http.Error(w, "缺少Device-Id或Client-Id", http.StatusBadRequest)
		return
	}

	//deviceId = strings.ReplaceAll(deviceId, ":", "_")

	//根据ip选择不同的配置
	clientIp := r.Header.Get("X-Real-IP")
	if clientIp == "" {
		clientIp = r.Header.Get("X-Forwarded-For")
	}
	if clientIp == "" {
		clientIp = r.RemoteAddr
	}

	var activationInfo *ActivationInfo
	authEnable := viper.GetBool("auth.enable")
	log.Debugf("authEnable: %v", authEnable)
	if authEnable {
		configProvider, err := user_config.GetProvider(viper.GetString("config_provider.type"))
		//检查此deviceId是否已认证
		isActivited, err := configProvider.IsDeviceActivated(r.Context(), deviceId, clientId)
		if err != nil {
			log.Errorf("检查设备是否认证失败: %v", err)
			http.Error(w, "内部服务器错误", http.StatusInternalServerError)
			return
		}
		if !isActivited {
			code, challenge, msg, timeoutMs := configProvider.GetActivationInfo(r.Context(), deviceId, clientId)
			// code现在返回的是int类型，但实际上后端API返回字符串
			// 为了保持兼容性，这里仍然格式化为字符串
			activationInfo = &ActivationInfo{
				Code:      fmt.Sprintf("%d", code),
				Message:   msg,
				Challenge: challenge,
				TimeoutMs: timeoutMs,
			}
			log.Infof("激活信息: &{Code:%d Message:%s Challenge:%s TimeoutMs:%d}", code, msg, challenge, timeoutMs)
		}
	}

	otaConfigPrefix := "ota.external."
	//如果ip是192.168开头的，则选择test配置
	if strings.HasPrefix(clientIp, "192.168") || strings.HasPrefix(clientIp, "10.") || strings.HasPrefix(clientIp, "127.0.0.1") {
		otaConfigPrefix = "ota.test."
	} else {
		otaConfigPrefix = "ota.external."
	}

	mqttInfo := getMqttInfo(deviceId, clientId, otaConfigPrefix, ip)
	//密码
	respData := &OtaResponse{
		Websocket: WebsocketInfo{
			Url:   viper.GetString(otaConfigPrefix + "websocket.url"),
			Token: viper.GetString(otaConfigPrefix + "websocket.token"),
		},
		Mqtt: mqttInfo,
		ServerTime: ServerTimeInfo{
			Timestamp:      time.Now().UnixMilli(),
			TimezoneOffset: 480,
		},
		Activation: activationInfo,
		Firmware: FirmwareInfo{
			Version: "0.9.9",
			Url:     "",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(respData); err != nil {
		log.Errorf("OTA响应序列化失败: %v", err)
		http.Error(w, "内部服务器错误", http.StatusInternalServerError)
		return
	}
	return
}

func getMqttInfo(deviceId, clientId, otaConfigPrefix, ip string) *MqttInfo {
	if !viper.GetBool(otaConfigPrefix + "mqtt.enable") {
		return nil
	}

	// 生成MQTT凭据
	signatureKey := viper.GetString("ota.signature_key")
	credentials, err := util.GenerateMqttCredentials(deviceId, clientId, ip, signatureKey)
	if err != nil {
		log.Errorf("生成MQTT凭据失败: %v", err)
		return nil
	}

	// 判断是否使用内置MQTT服务器(有hook功能)还是外部MQTT broker
	useMqttServer := viper.GetBool("mqtt_server.enable")

	var publishTopic, subscribeTopic string
	if useMqttServer {
		// 内置MQTT服务器: 使用 device-server,由hook自动重写为 /p2p/device_public/{mac}
		publishTopic = client.DeviceMockPubTopicPrefix   // "device-server"
		subscribeTopic = client.DeviceMockSubTopicPrefix // "null"
	} else {
		// 外部MQTT broker: 直接使用完整主题,避免所有设备消息混在一起
		// 从clientId中解析MAC地址: "GID_test@@@b4_3a_45_5a_3e_98@@@xxx" -> "b4_3a_45_5a_3e_98"
		macAddr := strings.ReplaceAll(deviceId, ":", "_")
		publishTopic = fmt.Sprintf("%s%s", client.DevicePubTopicPrefix, macAddr)     // "/p2p/device_public/{mac}"
		subscribeTopic = fmt.Sprintf("%s%s", client.DeviceSubTopicPrefix, macAddr)   // "/p2p/device_sub/{mac}"
	}

	return &MqttInfo{
		Endpoint:       viper.GetString(otaConfigPrefix + "mqtt.endpoint"),
		ClientId:       credentials.ClientId,
		Username:       credentials.Username,
		Password:       credentials.Password,
		PublishTopic:   publishTopic,
		SubscribeTopic: subscribeTopic,
	}
}

// handleOtaActivate 设备激活接口
func (s *WebSocketServer) handleOtaActivate(w http.ResponseWriter, r *http.Request) {
	deviceId := r.Header.Get("Device-Id")
	clientId := r.Header.Get("Client-Id")
	if deviceId == "" || clientId == "" {
		log.Errorf("缺少Device-Id或Client-Id")
		http.Error(w, "缺少Device-Id或Client-Id", http.StatusBadRequest)
		return
	}
	var req ActivationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Errorf("激活请求解析失败: %v", err)
		http.Error(w, "请求体解析失败", http.StatusBadRequest)
		return
	}
	// 校验算法
	if req.Payload.Algorithm != "hmac-sha256" {
		http.Error(w, "不支持的算法", http.StatusBadRequest)
		return
	}

	// 调用配置Provider进行绑定校验
	configProvider, err := user_config.GetProvider(viper.GetString("config_provider.type"))
	if err != nil {
		log.Errorf("获取配置Provider失败: %v", err)
		http.Error(w, "内部服务器错误", http.StatusInternalServerError)
		return
	}
	ok, err := configProvider.VerifyChallenge(r.Context(), deviceId, clientId, req.Payload)
	if err != nil {
		log.Errorf("设备激活校验失败: %v", err)
		http.Error(w, "设备激活校验失败", http.StatusInternalServerError)
		return
	}
	if !ok {
		log.Warnf("设备激活校验未通过: deviceId=%s, clientId=%s", deviceId, clientId)
		http.Error(w, "设备激活校验未通过", http.StatusAccepted)
		return
	}
	// 激活成功，返回200
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("激活成功"))
}
