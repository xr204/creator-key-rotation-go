package rotation

import (
	"context"
	"fmt"
)

type RotationRequest struct {
	KeyID          string
	GraceHours     int
	IdempotencyKey string
}

type RotationResult struct {
	KeyID              string `json:"key_id"`
	GraceHours         int    `json:"grace_hours"`
	DeliveryState      string `json:"delivery_state"`
	SubscriberState    string `json:"subscriber_state"`
	ContentWorkerState string `json:"content_worker_state"`
	LogSearchState     string `json:"log_search_state"`
}

type rotationAPI interface {
	RotateTemporaryKey(context.Context, string, int, string) error
	SearchDeploymentLogs(context.Context) error
}

func RotateCreatorKey(ctx context.Context, api rotationAPI, request RotationRequest) (RotationResult, error) {
	if request.KeyID == "" || request.IdempotencyKey == "" {
		return RotationResult{}, fmt.Errorf("key id and idempotency key are required")
	}
	if request.GraceHours <= 0 {
		return RotationResult{}, fmt.Errorf("grace hours must be positive")
	}
	if err := api.RotateTemporaryKey(ctx, request.KeyID, request.GraceHours, request.IdempotencyKey); err != nil {
		return RotationResult{}, err
	}
	if err := api.SearchDeploymentLogs(ctx); err != nil {
		return RotationResult{}, err
	}
	return RotationResult{
		KeyID: request.KeyID, GraceHours: request.GraceHours,
		DeliveryState:      "old key remains valid during grace period",
		SubscriberState:    "subscriber updates continue during grace period",
		ContentWorkerState: "content processing continues during grace period",
		LogSearchState:     "deployment logs searched with the same credential",
	}, nil
}
