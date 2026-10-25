package usage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ozine/videohelper/internal/usage/usagedb"
)

// RecordIndexed shares the final stage transaction, whose attempt fence prevents double counting.
func RecordIndexed(ctx context.Context, tx pgx.Tx, workspaceID, videoID int64, durationMs int) error {
	return usagedb.New(tx).RecordIndexed(ctx, usagedb.RecordIndexedParams{WorkspaceID: workspaceID, Amount: int64(durationMs), Ref: pgtype.Text{String: fmt.Sprintf("video:%d", videoID), Valid: true}})
}
