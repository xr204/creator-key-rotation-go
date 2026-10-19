package rotation

import (
	"context"
	"testing"
)

type fakeRotationAPI struct {
	rotated  bool
	searched bool
}

func (f *fakeRotationAPI) RotateTemporaryKey(context.Context, string, int, string) error {
	f.rotated = true
	return nil
}
func (f *fakeRotationAPI) SearchDeploymentLogs(context.Context) error {
	f.searched = true
	return nil
}

func TestRotateCreatorKeyKeepsWorkloadsInGracePeriod(t *testing.T) {
	api := &fakeRotationAPI{}
	result, err := RotateCreatorKey(context.Background(), api, RotationRequest{
		KeyID: "existing-key-42", GraceHours: 24, IdempotencyKey: "rotation-42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !api.rotated || !api.searched {
		t.Fatalf("workflow calls: rotate=%v logs=%v", api.rotated, api.searched)
	}
	if result.DeliveryState != "old key remains valid during grace period" || result.LogSearchState != "deployment logs searched with the same credential" {
		t.Fatalf("unexpected workflow result: %+v", result)
	}
}
