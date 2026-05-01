package main

import (
	"jobrunner/internal/app"
	"log"
)

func main() {
	a, err := app.New("config.yaml")
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}

	if err := a.Run(); err != nil {
		log.Fatalf("failed to run app: %v", err)
	}
}
