package index

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/index/indexdb"
	"github.com/ozine/videohelper/internal/platform/db"
)

func DeleteVideoChannel(ctx context.Context, q db.Querier, videoID int64, channel string) error {
	return indexdb.New(q).DeleteVideoChannel(ctx, indexdb.DeleteVideoChannelParams{VideoID: videoID, Channel: channel})
}

func DeleteVideo(ctx context.Context, q db.Querier, videoID int64) error {
	return indexdb.New(q).DeleteVideo(ctx, videoID)
}

func Coverage(ctx context.Context, q db.Querier, channel, spaceID string) (int, error) {
	n, err := indexdb.New(q).Coverage(ctx, indexdb.CoverageParams{Channel: channel, SpaceID: spaceID})
	return int(n), err
}

func DropSpace(ctx context.Context, pool *pgxpool.Pool, spaceID string, batch int) error {
	if err := literal(spaceID); err != nil {
		return err
	}
	if batch <= 0 || batch > math.MaxInt32 {
		return apperr.New(apperr.Invalid, "invalid_argument", "删除批量必须是正整数")
	}
	q := indexdb.New(pool)
	if err := q.DisableSpace(ctx, spaceID); err != nil {
		return err
	}
	for {
		n, err := q.DeleteSpaceBatch(ctx, indexdb.DeleteSpaceBatchParams{SpaceID: spaceID, Limit: int32(batch)})
		if err != nil {
			return err
		}
		if n == 0 {
			break
		}
	}
	for _, channel := range []string{"visual", "speech", "audio"} {
		name := pgx.Identifier{indexName(channel, spaceID)}.Sanitize()
		if _, err := pool.Exec(ctx, "DROP INDEX CONCURRENTLY IF EXISTS "+name); err != nil {
			return err
		}
	}
	return nil
}
