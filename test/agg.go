package main

import (
	"fmt"
	"io"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	fmt.Println("=== New Request ===")
	fmt.Println("Method:", r.Method)
	fmt.Println("URL:", r.URL)
	fmt.Println("Headers:", r.Header)
	fmt.Println("Body:", string(body))
	w.Write([]byte("Got it!\n"))
}

func main() {
	http.HandleFunc("/", handler)
	fmt.Println("Listening on :8000")
	http.ListenAndServe(":8000", nil)
}
