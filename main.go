package main

import (
	"fmt"
	"net/http"
)

func main() {
	httpServeMux := http.NewServeMux()
	server := http.Server{}
	server.Handler = httpServeMux
	server.Addr = ":8080"

	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
	}
}
