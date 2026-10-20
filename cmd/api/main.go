package main

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/zeromicro/go-zero/rest"
	"goguide/internal/api"
)

func main() {
	secret := os.Getenv("GOGUIDE_JWT_SECRET")
	if len(secret) < 32 {
		log.Fatal("GOGUIDE_JWT_SECRET must contain at least 32 characters")
	}
	db, err := api.OpenDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	addr := os.Getenv("GOGUIDE_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	host, portText, ok := strings.Cut(addr, ":")
	if !ok {
		log.Fatal("GOGUIDE_ADDR must be host:port")
	}
	if host == "" {
		host = "0.0.0.0"
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		log.Fatal(err)
	}
	conf := rest.RestConf{Host: host, Port: port, Timeout: 20000}
	conf.Name = "goguide-api"
	srv := rest.MustNewServer(conf)
	defer srv.Stop()
	api.New(db, secret).Routes(srv)
	log.Printf("GoGuide API listening on %s", addr)
	srv.Start()
}
