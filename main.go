package main

import (
	"flag"
	"log"
	"os"

	"github.com/Salastil/streamed-tui/internal"
)

func main() {
	embedURL := flag.String("e", "", "extract a single embed URL and launch mpv")
	debug := flag.Bool("debug", false, "enable verbose extractor/debug output")
	flag.Parse()

	if *embedURL != "" {
		if err := internal.RunExtractorCLI(*embedURL, *debug); err != nil {
			log.Println("error:", err)
			os.Exit(1)
		}
		return
	}

	if err := internal.Run(*debug); err != nil {
		log.Println("error:", err)
		os.Exit(1)
	}
}
