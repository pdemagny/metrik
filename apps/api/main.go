package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	router, err := NewRouter()
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Addr:        ":8080",
		Handler:     router,
		ReadTimeout: 10 * time.Second,
	}
	err = srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
