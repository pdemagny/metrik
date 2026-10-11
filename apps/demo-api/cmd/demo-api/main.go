package main

import (
	"log"
	"net/http"
	"time"

	"demo-api/internal/router"
)

func main() {
	r, err := router.NewRouter()
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Addr:        ":8080",
		Handler:     r,
		ReadTimeout: 10 * time.Second,
	}
	err = srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
