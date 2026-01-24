package main

import (
	"log"
	"net/http"
)

func main() {
	router, err := NewRouter()
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}
	err = srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
