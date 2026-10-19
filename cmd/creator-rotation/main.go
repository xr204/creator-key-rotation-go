package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"example.com/creator-key-rotation/rotation"
)

func main() {
	client, err := rotation.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	result, err := rotation.RotateCreatorKey(context.Background(), client, rotation.RotationRequest{
		KeyID: os.Getenv("ROTATION_KEY_ID"), GraceHours: 24,
		IdempotencyKey: os.Getenv("ROTATION_IDEMPOTENCY_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		log.Fatal(err)
	}
}
