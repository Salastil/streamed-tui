package main

import (
	"log"
	"os"

	"github.com/Salastil/streamed-tui/internal"
)

func main() {
	if err := internal.Run(); err != nil {
		log.Println("error:", err)
		os.Exit(1)
	}
}
