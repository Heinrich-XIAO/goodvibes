// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hackclub/goodvibes/internal/db"
	"github.com/hackclub/goodvibes/internal/server"
)

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

func main() {
	loadDotEnv(".env")

	dbPath := os.Getenv("GOODVIBES_DB")
	if dbPath == "" {
		dbPath = "data/goodvibes.db"
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	handler, _ := server.NewRouter(conn)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("goodvibes-go (Hackatime Go port) listening on http://localhost:%s (db: %s)", port, dbPath)
	log.Fatal(srv.ListenAndServe())
}
