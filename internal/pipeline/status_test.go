package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestScopedPublicStageStatusIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
	if err := service.Start(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	row := onlyStage(t, ctx, service, videoID, "probe")
	_, p := taskForRow(t, workspaceID, row)
	sc, err := service.claim(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	progress := map[string]any{"next": 2, "total": 4, "raw_keys": []string{"private/artifact"}, "plan": map[string]string{"version": "cannot-overwrite-plan"}}
	if err := service.saveProgress(ctx, p, sc.Attempt, progress, nil); err != nil {
		t.Fatal(err)
	}
	stages, err := service.ListStages(ctx, workspaceID, videoID)
	if err != nil || len(stages) != 1 || stages[0].Stage != "probe" || stages[0].Version != "v1" || stages[0].Attempt != 1 || stages[0].Status != "running" || stages[0].Progress == nil || *stages[0].Progress != 0.5 {
		t.Fatalf("public stage status=%+v error=%v", stages, err)
	}
	body, err := json.Marshal(stages)
	if err != nil || strings.Contains(string(body), "private/artifact") || strings.Contains(string(body), row.Version) || strings.Contains(string(body), "raw_keys") {
		t.Fatalf("public stage status leaked checkpoint details: %s error=%v", body, err)
	}
	for _, input := range []map[string]int{{"next": 5, "total": 4}, {"next": 0}, {"total": 4}} {
		if err := service.saveProgress(ctx, p, sc.Attempt, input, nil); err != nil {
			t.Fatal(err)
		}
		stages, err := service.ListStages(ctx, workspaceID, videoID)
		if err != nil || len(stages) != 1 || stages[0].Progress != nil {
			t.Fatalf("invalid progress exposed as a fraction: %+v error=%v", stages, err)
		}
	}
	for _, scope := range []struct{ workspace, video int64 }{{workspaceID + 1, videoID}, {workspaceID, videoID + 1000}} {
		stages, err := service.ListStages(ctx, scope.workspace, scope.video)
		if err != nil || len(stages) != 0 || stages == nil {
			t.Fatalf("foreign/missing stage status=%+v error=%v", stages, err)
		}
	}
	if _, err := service.deps.Pool.Exec(ctx, "UPDATE video SET deleted_at=now() WHERE id=$1", videoID); err != nil {
		t.Fatal(err)
	}
	if stages, err := service.ListStages(ctx, workspaceID, videoID); err != nil || len(stages) != 0 {
		t.Fatalf("deleted stage status=%+v error=%v", stages, err)
	}
}
