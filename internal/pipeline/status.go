package pipeline

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
)

type StageStatus struct {
	Stage     string    `json:"stage"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Attempt   int       `json:"attempt"`
	Required  bool      `json:"required"`
	Progress  *float64  `json:"progress,omitempty"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Service) ListStages(ctx context.Context, workspaceID, videoID int64) ([]StageStatus, error) {
	rows, err := pipelinedb.New(s.deps.Pool).ListStages(ctx, pipelinedb.ListStagesParams{VideoID: videoID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	stages := make([]StageStatus, 0, len(rows))
	for _, row := range rows {
		output, err := decodeOutput(row.Output)
		if err != nil {
			return nil, err
		}
		stage := StageStatus{Stage: row.Stage, Version: output.Plan.Version, Status: row.Status, Attempt: int(row.Attempt), Required: row.Required,
			Error: row.Error.String, UpdatedAt: row.UpdatedAt.Time}
		var progress struct{ Next, Total *float64 }
		if json.Unmarshal(output.Progress, &progress) == nil && progress.Next != nil && progress.Total != nil && *progress.Total > 0 && *progress.Next >= 0 && *progress.Next <= *progress.Total {
			fraction := *progress.Next / *progress.Total
			stage.Progress = &fraction
		}
		stages = append(stages, stage)
	}
	return stages, nil
}
