package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// 迁移配置
type Config struct {
	// MySQL 配置
	MySQLHost     string
	MySQLPort     string
	MySQLUser     string
	MySQLPassword string
	MySQLDatabase string

	// PostgreSQL 配置
	PGHost     string
	PGPort     string
	PGUser     string
	PGPassword string
	PGDatabase string
	PGSSLMode  string

	// 迁移选项
	DryRun    bool
	BatchSize int
}

func main() {
	config := parseFlags()

	log.Println("Starting MySQL to PostgreSQL migration...")

	// 连接 MySQL
	mysqlDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True",
		config.MySQLUser, config.MySQLPassword, config.MySQLHost, config.MySQLPort, config.MySQLDatabase)
	mysqlDB, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}
	defer mysqlDB.Close()

	// 连接 PostgreSQL
	pgDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.PGHost, config.PGPort, config.PGUser, config.PGPassword, config.PGDatabase, config.PGSSLMode)
	pgDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pgDB.Close()

	// 测试连接
	if err := mysqlDB.Ping(); err != nil {
		log.Fatalf("Failed to ping MySQL: %v", err)
	}
	log.Println("Connected to MySQL successfully")

	if err := pgDB.Ping(); err != nil {
		log.Fatalf("Failed to ping PostgreSQL: %v", err)
	}
	log.Println("Connected to PostgreSQL successfully")

	if config.DryRun {
		log.Println("DRY RUN MODE - No data will be written")
	}

	// 迁移各表数据
	tables := []string{"users", "devices", "agents", "configs", "global_roles"}
	for _, table := range tables {
		if err := migrateTable(mysqlDB, pgDB, table, config); err != nil {
			log.Printf("Warning: Failed to migrate table %s: %v", table, err)
		}
	}

	log.Println("Migration completed!")
}

func parseFlags() *Config {
	config := &Config{}

	// MySQL flags
	flag.StringVar(&config.MySQLHost, "mysql-host", "localhost", "MySQL host")
	flag.StringVar(&config.MySQLPort, "mysql-port", "3306", "MySQL port")
	flag.StringVar(&config.MySQLUser, "mysql-user", "root", "MySQL user")
	flag.StringVar(&config.MySQLPassword, "mysql-password", "", "MySQL password")
	flag.StringVar(&config.MySQLDatabase, "mysql-db", "xiaozhi_admin", "MySQL database")

	// PostgreSQL flags
	flag.StringVar(&config.PGHost, "pg-host", "localhost", "PostgreSQL host")
	flag.StringVar(&config.PGPort, "pg-port", "5432", "PostgreSQL port")
	flag.StringVar(&config.PGUser, "pg-user", "xiaozhi", "PostgreSQL user")
	flag.StringVar(&config.PGPassword, "pg-password", "xiaozhi_password", "PostgreSQL password")
	flag.StringVar(&config.PGDatabase, "pg-db", "xiaozhi_admin", "PostgreSQL database")
	flag.StringVar(&config.PGSSLMode, "pg-sslmode", "disable", "PostgreSQL SSL mode")

	// Migration options
	flag.BoolVar(&config.DryRun, "dry-run", false, "Dry run mode")
	flag.IntVar(&config.BatchSize, "batch-size", 1000, "Batch size for migration")

	flag.Parse()
	return config
}

func migrateTable(mysqlDB, pgDB *sql.DB, tableName string, config *Config) error {
	log.Printf("Migrating table: %s", tableName)

	// 获取 MySQL 记录数
	var count int
	if err := mysqlDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count); err != nil {
		return fmt.Errorf("failed to count rows: %w", err)
	}
	log.Printf("  Found %d rows in MySQL", count)

	if count == 0 {
		log.Printf("  Skipping empty table")
		return nil
	}

	// 根据表名执行不同的迁移逻辑
	switch tableName {
	case "users":
		return migrateUsers(mysqlDB, pgDB, config)
	case "devices":
		return migrateDevices(mysqlDB, pgDB, config)
	case "agents":
		return migrateAgents(mysqlDB, pgDB, config)
	case "configs":
		return migrateConfigs(mysqlDB, pgDB, config)
	case "global_roles":
		return migrateGlobalRoles(mysqlDB, pgDB, config)
	default:
		return fmt.Errorf("unknown table: %s", tableName)
	}
}

func migrateUsers(mysqlDB, pgDB *sql.DB, config *Config) error {
	rows, err := mysqlDB.Query("SELECT id, username, password, email, role, created_at, updated_at FROM users")
	if err != nil {
		return err
	}
	defer rows.Close()

	var migrated int
	for rows.Next() {
		var id int64
		var username, password, role string
		var email sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &username, &password, &email, &role, &createdAt, &updatedAt); err != nil {
			log.Printf("  Warning: Failed to scan user row: %v", err)
			continue
		}

		if !config.DryRun {
			_, err := pgDB.Exec(`
				INSERT INTO users (id, username, password, email, role, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (id) DO UPDATE SET
					username = EXCLUDED.username,
					password = EXCLUDED.password,
					email = EXCLUDED.email,
					role = EXCLUDED.role,
					updated_at = EXCLUDED.updated_at
			`, id, username, password, email, role, createdAt, updatedAt)
			if err != nil {
				log.Printf("  Warning: Failed to insert user %d: %v", id, err)
				continue
			}
		}
		migrated++
	}

	// 更新序列
	if !config.DryRun {
		pgDB.Exec("SELECT setval('users_id_seq', (SELECT MAX(id) FROM users))")
	}

	log.Printf("  Migrated %d users", migrated)
	return nil
}

func migrateDevices(mysqlDB, pgDB *sql.DB, config *Config) error {
	rows, err := mysqlDB.Query(`
		SELECT id, user_id, agent_id, device_code, device_name, challenge,
		       pre_secret_key, activated, last_active_at, created_at, updated_at
		FROM devices
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var migrated int
	for rows.Next() {
		var id, userID, agentID int64
		var deviceCode, deviceName, challenge, preSecretKey sql.NullString
		var activated bool
		var lastActiveAt sql.NullTime
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &userID, &agentID, &deviceCode, &deviceName, &challenge,
			&preSecretKey, &activated, &lastActiveAt, &createdAt, &updatedAt); err != nil {
			log.Printf("  Warning: Failed to scan device row: %v", err)
			continue
		}

		if !config.DryRun {
			_, err := pgDB.Exec(`
				INSERT INTO devices (id, user_id, agent_id, device_code, device_name, challenge,
				                     pre_secret_key, activated, last_active_at, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				ON CONFLICT (id) DO UPDATE SET
					user_id = EXCLUDED.user_id,
					agent_id = EXCLUDED.agent_id,
					device_code = EXCLUDED.device_code,
					device_name = EXCLUDED.device_name,
					activated = EXCLUDED.activated,
					last_active_at = EXCLUDED.last_active_at,
					updated_at = EXCLUDED.updated_at
			`, id, userID, agentID, deviceCode, deviceName, challenge,
				preSecretKey, activated, lastActiveAt, createdAt, updatedAt)
			if err != nil {
				log.Printf("  Warning: Failed to insert device %d: %v", id, err)
				continue
			}
		}
		migrated++
	}

	if !config.DryRun {
		pgDB.Exec("SELECT setval('devices_id_seq', (SELECT MAX(id) FROM devices))")
	}

	log.Printf("  Migrated %d devices", migrated)
	return nil
}

func migrateAgents(mysqlDB, pgDB *sql.DB, config *Config) error {
	rows, err := mysqlDB.Query(`
		SELECT id, user_id, name, custom_prompt, llm_config_id, tts_config_id,
		       asr_speed, status, created_at, updated_at
		FROM agents
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var migrated int
	for rows.Next() {
		var id, userID int64
		var name string
		var customPrompt, llmConfigID, ttsConfigID, asrSpeed, status sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &userID, &name, &customPrompt, &llmConfigID, &ttsConfigID,
			&asrSpeed, &status, &createdAt, &updatedAt); err != nil {
			log.Printf("  Warning: Failed to scan agent row: %v", err)
			continue
		}

		if !config.DryRun {
			_, err := pgDB.Exec(`
				INSERT INTO agents (id, user_id, name, custom_prompt, llm_config_id, tts_config_id,
				                    asr_speed, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (id) DO UPDATE SET
					user_id = EXCLUDED.user_id,
					name = EXCLUDED.name,
					custom_prompt = EXCLUDED.custom_prompt,
					llm_config_id = EXCLUDED.llm_config_id,
					tts_config_id = EXCLUDED.tts_config_id,
					asr_speed = EXCLUDED.asr_speed,
					status = EXCLUDED.status,
					updated_at = EXCLUDED.updated_at
			`, id, userID, name, customPrompt, llmConfigID, ttsConfigID,
				asrSpeed, status, createdAt, updatedAt)
			if err != nil {
				log.Printf("  Warning: Failed to insert agent %d: %v", id, err)
				continue
			}
		}
		migrated++
	}

	if !config.DryRun {
		pgDB.Exec("SELECT setval('agents_id_seq', (SELECT MAX(id) FROM agents))")
	}

	log.Printf("  Migrated %d agents", migrated)
	return nil
}

func migrateConfigs(mysqlDB, pgDB *sql.DB, config *Config) error {
	rows, err := mysqlDB.Query(`
		SELECT id, type, name, config_id, provider, json_data,
		       enabled, is_default, created_at, updated_at
		FROM configs
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var migrated int
	for rows.Next() {
		var id int64
		var configType, name, configID string
		var provider, jsonData sql.NullString
		var enabled, isDefault bool
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &configType, &name, &configID, &provider, &jsonData,
			&enabled, &isDefault, &createdAt, &updatedAt); err != nil {
			log.Printf("  Warning: Failed to scan config row: %v", err)
			continue
		}

		if !config.DryRun {
			_, err := pgDB.Exec(`
				INSERT INTO configs (id, type, name, config_id, provider, json_data,
				                     enabled, is_default, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (id) DO UPDATE SET
					type = EXCLUDED.type,
					name = EXCLUDED.name,
					config_id = EXCLUDED.config_id,
					provider = EXCLUDED.provider,
					json_data = EXCLUDED.json_data,
					enabled = EXCLUDED.enabled,
					is_default = EXCLUDED.is_default,
					updated_at = EXCLUDED.updated_at
			`, id, configType, name, configID, provider, jsonData,
				enabled, isDefault, createdAt, updatedAt)
			if err != nil {
				log.Printf("  Warning: Failed to insert config %d: %v", id, err)
				continue
			}
		}
		migrated++
	}

	if !config.DryRun {
		pgDB.Exec("SELECT setval('configs_id_seq', (SELECT MAX(id) FROM configs))")
	}

	log.Printf("  Migrated %d configs", migrated)
	return nil
}

func migrateGlobalRoles(mysqlDB, pgDB *sql.DB, config *Config) error {
	rows, err := mysqlDB.Query(`
		SELECT id, name, description, prompt, is_default, created_at, updated_at
		FROM global_roles
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var migrated int
	for rows.Next() {
		var id int64
		var name string
		var description, prompt sql.NullString
		var isDefault bool
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &description, &prompt, &isDefault, &createdAt, &updatedAt); err != nil {
			log.Printf("  Warning: Failed to scan global_role row: %v", err)
			continue
		}

		if !config.DryRun {
			_, err := pgDB.Exec(`
				INSERT INTO global_roles (id, name, description, prompt, is_default, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (id) DO UPDATE SET
					name = EXCLUDED.name,
					description = EXCLUDED.description,
					prompt = EXCLUDED.prompt,
					is_default = EXCLUDED.is_default,
					updated_at = EXCLUDED.updated_at
			`, id, name, description, prompt, isDefault, createdAt, updatedAt)
			if err != nil {
				log.Printf("  Warning: Failed to insert global_role %d: %v", id, err)
				continue
			}
		}
		migrated++
	}

	if !config.DryRun {
		pgDB.Exec("SELECT setval('global_roles_id_seq', (SELECT MAX(id) FROM global_roles))")
	}

	log.Printf("  Migrated %d global_roles", migrated)
	return nil
}
