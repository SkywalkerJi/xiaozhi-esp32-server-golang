  ✅ 服务状态

  1. MySQL 数据库: 127.0.0.1:3306 ✓
  2. 管理后端: 127.0.0.1:8080 ✓
  3. 前端: localhost:3000 ✓
  4. 主服务器:
    - WebSocket: 8989 ✓
    - MQTT: 2883 ✓
    - UDP: 8990 ✓

  🎯 问题原因

  你的项目包含两个独立的服务：

  1. 主服务器 (./xiaozhi_server) - 提供 WebSocket/MQTT/UDP 服务，用于设备连接
  2. 管理后端 (manager/backend/xiaozhi_backend) - 提供 HTTP API 服务，用于前端管理界面

  之前只启动了主服务器，但没有启动管理后端，导致前端无法访问8080端口的API。

  📋 后续使用

  现在前端应该可以正常访问了。打开浏览器访问 http://localhost:3000，你应该会看到一个初始化引导页面，因为数据库表结构还未初始化。

  下次启动服务的完整流程：

  # 1. 启动 MySQL（如果容器存在）
  docker start xiaozhi-mysql

  # 2. 启动管理后端（在 manager/backend 目录）
  cd manager/backend
  ./xiaozhi_backend -config config/config.json &

  # 3. 启动主服务器（在项目根目录）
  cd ../../
  ./xiaozhi_server -c config/config.yaml &

  # 4. 启动前端（在 manager/frontend 目录）
  cd manager/frontend
  npm run dev
