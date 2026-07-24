package main

import (
	"fmt"
	"log"

	"github.com/songguangzhi/webhook-ui/internal/database"
)

func main() {
	if err := database.Init("./data"); err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	fmt.Println("database initialized")
}
