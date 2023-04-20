package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/rand"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	list := []int{}
	for i := 0; i < 100000000; i++ {
		list = append(list, rand.Int())
	}
	fmt.Fprintf(w, "Hi, I'm a loadtester server")
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8080", nil)
}
