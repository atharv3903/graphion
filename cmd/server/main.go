package main

import (
	"database/sql"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
	"github.com/atharv3903/graphion/internal/api"
	"github.com/atharv3903/graphion/internal/config"
)

func main() {
	cfg := config.FromFlagsServer()

	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	srv := api.New(db)

	log.Println("GRAPHION listening on", cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, srv.Mux))
}
