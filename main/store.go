package main

import (
	"github.com/jacklaaa89/queue/store"
)

func main() {
	server, err := store.New(store.NewDefaultConfig())
	if err != nil {
		panic(err)
	}

	server.Listen()
}
