package mcp

import (
	"context"
	"fmt"
	"log"
	"time"

	"xiaozhi-esp32-server-golang/logger"

	"github.com/cloudwego/eino/components/tool"
	"github.com/gorilla/websocket"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	cmap "github.com/orcaman/concurrent-map/v2"
)

// DeviceMcpSession 代表一个设备的MCP会话，聚合了多种MCP连接
type DeviceMcpSession struct {
	deviceID      string
	Ctx           context.Context
	cancel        context.CancelFunc
	wsEndPointMcp cmap.ConcurrentMap[string, *McpClientInstance]
	iotOverMcp    *McpClientInstance
}

func (dcs *DeviceMcpSession) AddWsEndPointMcp(mcpClient *McpClientInstance) {
	dcs.wsEndPointMcp.Set(mcpClient.serverName, mcpClient)

	// 设置关闭回调
	mcpClient.SetOnCloseHandler(dcs.handleMcpClientClose)

	mcpClient.refreshTools()
}

// todo
func (dcs *DeviceMcpSession) SetIotOverMcp(mcpClient *McpClientInstance) {
	dcs.iotOverMcp = mcpClient

	// 设置关闭回调
	mcpClient.SetOnCloseHandler(dcs.handleMcpClientClose)

	mcpClient.refreshTools()
}

func (dcs *DeviceMcpSession) RemoveWsEndPointMcp(mcpClient *McpClientInstance) {
	dcs.wsEndPointMcp.Remove(mcpClient.serverName)
}

// GetDeviceID 获取设备ID
func (dcs *DeviceMcpSession) GetDeviceID() string {
	return dcs.deviceID
}

// handleMcpClientClose 处理MCP客户端关闭事件
func (dcs *DeviceMcpSession) handleMcpClientClose(instance *McpClientInstance, reason string) {
	logger.Infof("设备 %s 的MCP客户端 %s 已关闭，原因: %s", dcs.deviceID, instance.serverName, reason)

	// 从会话中移除已关闭的客户端
	dcs.RemoveWsEndPointMcp(instance)

	// 如果所有WebSocket端点都关闭了，可以考虑清理整个会话
	/*if dcs.wsEndPointMcp.Count() == 0 && dcs.iotOverMcp == nil {
		logger.Infof("设备 %s 的所有MCP连接已关闭，清理会话", dcs.deviceID)
		dcs.cancel()
	}*/
}

// McpClientInstance 代表一个具体的MCP客户端连接
type McpClientInstance struct {
	serverName string
	mcpClient  *client.Client // 是从ws endpoint连上来的mcp server
	tools      cmap.ConcurrentMap[string, tool.InvokableTool]
	serverInfo *mcp.InitializeResult
	lastPing   time.Time
	Ctx        context.Context
	cancel     context.CancelFunc
	connected  bool
	conn       ConnInterface

	// 添加关闭回调
	onCloseHandler func(instance *McpClientInstance, reason string)
}

// NewDeviceMCPClient 创建新的MCP客户端
func NewDeviceMCPSession(deviceID string) *DeviceMcpSession {
	ctx, cancel := context.WithCancel(context.Background())

	deviceMcpClient := &DeviceMcpSession{
		deviceID:      deviceID,
		Ctx:           ctx,
		cancel:        cancel,
		wsEndPointMcp: cmap.New[*McpClientInstance](),
	}

	go deviceMcpClient.refreshToolsAndPing()

	return deviceMcpClient
}

func NewWsEndPointMcpClient(ctx context.Context, deviceID string, conn *websocket.Conn) *McpClientInstance {
	ctx, cancel := context.WithCancel(ctx)

	wsTransport, err := NewWebsocketTransport(conn)
	if err != nil {
		logger.Errorf("创建MCP客户端失败: %v", err)
		return nil
	}
	mcpClient := client.NewClient(wsTransport)

	wsEndPointMcp := &McpClientInstance{
		serverName: fmt.Sprintf("ws_endpoint_mcp_%s_%s", deviceID, conn.RemoteAddr().String()),
		mcpClient:  mcpClient,
		tools:      cmap.New[tool.InvokableTool](),
		Ctx:        ctx,
		cancel:     cancel,
		connected:  true,
		lastPing:   time.Now(),
	}
	mcpClient.OnNotification(wsEndPointMcp.handleJSONRPCNotification)

	// 设置transport的关闭回调
	wsTransport.SetOnCloseHandler(wsEndPointMcp.handleTransportClose)

	wsEndPointMcp.sendInitlize(ctx)
	wsEndPointMcp.mcpClient.Start(ctx)
	return wsEndPointMcp
}

func NewIotOverMcpClient(deviceID string, conn ConnInterface) *McpClientInstance {
	ctx, cancel := context.WithCancel(context.Background())

	wsTransport, err := NewIotOverMcpTransport(conn)
	if err != nil {
		logger.Errorf("创建MCP客户端失败: %v", err)
		return nil
	}
	mcpClient := client.NewClient(wsTransport)

	iotOverMcp := &McpClientInstance{
		serverName: fmt.Sprintf("iot_over_mcp_%s", deviceID),
		mcpClient:  mcpClient,
		tools:      cmap.New[tool.InvokableTool](),
		Ctx:        ctx,
		cancel:     cancel,
		connected:  true,
		lastPing:   time.Now(),
	}
	wsTransport.SetNotificationHandler(iotOverMcp.handleJSONRPCNotification)

	// 设置transport的关闭回调
	wsTransport.SetOnCloseHandler(iotOverMcp.handleTransportClose)

	iotOverMcp.sendInitlize(ctx)
	iotOverMcp.mcpClient.Start(ctx)

	return iotOverMcp
}

// refreshToolsCommon 通用的工具列表刷新逻辑
func (dc *McpClientInstance) refreshTools() error {
	tools, err := dc.mcpClient.ListTools(dc.Ctx, mcp.ListToolsRequest{})
	if err != nil {
		logger.Errorf("刷新工具列表失败: %v", err)
		return err
	}

	// 清空现有工具并添加新工具
	dc.tools.Clear()
	for name, tool := range ConvertMcpToolListToInvokableToolList(tools.Tools, dc.serverName, dc.mcpClient) {
		dc.tools.Set(name, tool)
	}

	logger.Infof("刷新工具列表成功: %s 获取到 %d 个工具", dc.serverName, dc.tools.Count())
	return nil
}

func (dc *McpClientInstance) GetServerName() string {
	return dc.serverName
}

func (dc *DeviceMcpSession) refreshToolsAndPing() {
	// 只在初始化时获取一次工具列表
	findTools := func(mcpInstance *McpClientInstance) {
		if mcpInstance == nil {
			return
		}
		mcpInstance.refreshTools()
	}

	ping := func(mcpInstance *McpClientInstance) {
		if mcpInstance == nil {
			return
		}
		err := mcpInstance.mcpClient.Ping(mcpInstance.Ctx)
		if err == nil {
			mcpInstance.lastPing = time.Now()
			logger.Debugf("设备 %s ping成功", mcpInstance.serverName)
		} else {
			logger.Warnf("设备 %s ping失败: %v", mcpInstance.serverName, err)
		}
	}

	// 初始化时获取工具列表
	for _, mcpInstance := range dc.wsEndPointMcp.Items() {
		findTools(mcpInstance)
	}
	findTools(dc.iotOverMcp)

	// 每2分钟进行一次ping
	pingTick := time.NewTicker(2 * time.Minute)
	defer pingTick.Stop()

	for {
		select {
		case <-dc.Ctx.Done():
			logger.Infof("设备 %s 会话已取消，停止ping", dc.deviceID)
			return
		case <-pingTick.C:
			for _, mcpInstance := range dc.wsEndPointMcp.Items() {
				ping(mcpInstance)
			}
			//ping(dc.iotOverMcp)
		}
	}
}

func (dc *McpClientInstance) sendInitlize(ctx context.Context) error {
	initRequest := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "mcp-go",
				Version: "0.1.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	}

	serverInfo, err := dc.mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		fmt.Printf("Failed to initialize: %v", err)
		return err
	}
	dc.serverInfo = serverInfo
	return nil
}

func (dc *McpClientInstance) findTools() (*mcp.ListToolsResult, error) {
	tools, err := dc.mcpClient.ListTools(dc.Ctx, mcp.ListToolsRequest{})
	if err != nil {
		logger.Errorf("获取工具列表失败: %v", err)
		return nil, err
	}
	return tools, nil
}

// handleJSONRPCNotification 处理JSON-RPC通知
func (dc *McpClientInstance) handleJSONRPCNotification(notification mcp.JSONRPCNotification) {
	switch notification.Method {
	case "notifications/progress":
		//handleProgressNotification(notification)
	case "notifications/message":
		//handleMessageNotification(notification)
	case "notifications/resources/updated":
		//handleResourceUpdateNotification(notification)
	case "notifications/tools/updated":
		// 收到工具更新通知，刷新工具列表
		logger.Infof("收到工具更新通知，刷新工具列表")
		go dc.refreshToolsOnNotification()
	default:
		log.Printf("Unknown notification: %s", notification.Method)
	}
}

// refreshToolsOnNotification 基于通知刷新工具列表
func (dc *McpClientInstance) refreshToolsOnNotification() {
	// 添加短暂延迟避免频繁刷新
	time.Sleep(100 * time.Millisecond)
	dc.refreshTools()
}

// handleJSONRPCError 处理JSON-RPC错误
func (dc *McpClientInstance) handleJSONRPCError(errMsg mcp.JSONRPCError) error {
	logger.Errorf("收到MCP服务器错误: %+v", errMsg.Error)
	return nil
}

// handleTransportClose 处理transport层关闭事件
func (dc *McpClientInstance) handleTransportClose(reason string) {
	logger.Infof("MCP客户端 %s transport层关闭，原因: %s", dc.serverName, reason)

	// 标记连接已断开
	dc.connected = false

	// 取消上下文
	dc.cancel()

	// 通知上层处理
	if dc.onCloseHandler != nil {
		dc.onCloseHandler(dc, reason)
	}
}

// SetOnCloseHandler 设置关闭回调
func (dc *McpClientInstance) SetOnCloseHandler(handler func(instance *McpClientInstance, reason string)) {
	dc.onCloseHandler = handler
}

// IsConnected 检查连接是否仍然活跃
func (dc *McpClientInstance) IsConnected() bool {
	return dc.connected
}

// GetConnectionStatus 获取连接状态信息
func (dc *McpClientInstance) GetConnectionStatus() map[string]interface{} {
	toolsCount := dc.tools.Count()

	return map[string]interface{}{
		"server_name": dc.serverName,
		"connected":   dc.connected,
		"last_ping":   dc.lastPing,
		"tools_count": toolsCount,
	}
}

// GetTools 获取工具列表
func (dc *DeviceMcpSession) GetTools() map[string]tool.InvokableTool {
	tools := make(map[string]tool.InvokableTool)
	for _, mcpInstance := range dc.wsEndPointMcp.Items() {
		for k, v := range mcpInstance.tools.Items() {
			tools[k] = v
		}
	}
	if dc.iotOverMcp != nil {
		for k, v := range dc.iotOverMcp.tools.Items() {
			tools[k] = v
		}
	}
	return tools
}

func (dc *DeviceMcpSession) GetWsEndpointMcpTools() map[string]tool.InvokableTool {
	tools := make(map[string]tool.InvokableTool)
	for _, mcpInstance := range dc.wsEndPointMcp.Items() {
		for k, v := range mcpInstance.tools.Items() {
			tools[k] = v
		}
	}
	return tools
}

func (dc *DeviceMcpSession) GetToolByName(toolName string) (tool.InvokableTool, bool) {
	for _, mcpInstance := range dc.wsEndPointMcp.Items() {
		if tool, ok := mcpInstance.tools.Get(toolName); ok {
			return tool, true
		}
	}
	if dc.iotOverMcp != nil {
		if tool, ok := dc.iotOverMcp.tools.Get(toolName); ok {
			return tool, true
		}
	}
	return nil, false
}
