package config

import (
	"flag"
	"os"
)

type ServerConfig struct {
	MySQLDSN string
	Addr     string
}

func FromFlagsServer() ServerConfig {
	var dsn, addr string
	flag.StringVar(&dsn, "dsn", os.Getenv("DB_DSN"), "MySQL DSN")
	flag.StringVar(&addr, "addr", ":8080", "HTTP bind address")
	flag.Parse()

	return ServerConfig{
		MySQLDSN: dsn,
		Addr:     addr,
	}
}
