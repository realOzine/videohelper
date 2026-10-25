package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
)

type Role string

const (
	RoleAPI     Role = "api"
	RoleWorker  Role = "worker"
	RoleMigrate Role = "migrate"
	RoleEval    Role = "eval"
	RoleSpace   Role = "space"
)

func (c Config) Validate(role Role) error {
	var errs []error
	require := func(ok bool, field string) {
		if !ok {
			errs = append(errs, fmt.Errorf("%s is required or invalid", field))
		}
	}
	switch role {
	case RoleAPI, RoleWorker, RoleMigrate, RoleEval, RoleSpace:
	default:
		return fmt.Errorf("unknown application role %q", role)
	}
	require(strings.TrimSpace(c.Database.URL) != "", "database.url_env")
	require(c.Database.MaxConns > 0 && c.Database.MaxConns <= math.MaxInt32, "database.max_conns")
	if role == RoleMigrate {
		return errors.Join(errs...)
	}
	require(len(c.ML.Endpoints) > 0, "ml.endpoints")
	for _, endpoint := range c.ML.Endpoints {
		require(httpURL(endpoint), "ml.endpoints")
	}
	require(strings.TrimSpace(c.ML.Token) != "", "ml.token_env")
	require(c.ML.QueryTimeout > 0 && c.ML.EmbedTimeout > 0, "ml timeouts")
	if role == RoleSpace {
		return errors.Join(errs...)
	}
	require(c.S3.Bucket != "" && c.S3.Region != "", "s3.bucket / s3.region")
	require(c.S3.Endpoint == "" || httpURL(c.S3.Endpoint), "s3.endpoint")
	require(c.S3.PresignTTL > 0, "s3.presign_ttl")
	require(len(c.Channels) > 0, "channels_file")
	require(c.Search.K > 0 && c.Search.RRFK > 0 && c.Search.MaxLimit > 0 && c.Search.MergeGapCells >= 0, "search")
	if role == RoleEval {
		return errors.Join(errs...)
	}
	require(strings.TrimSpace(c.Redis.Addr) != "", "redis.addr_env")
	require(c.Limits.UploadPartBytes >= 5*1024*1024, "limits.upload_part_bytes")
	require(c.Limits.SoftDeletePurgeAfter > 0, "limits.soft_delete_purge_after")
	require(len(c.Limits.Plans) > 0, "limits.plans")
	for name, plan := range c.Limits.Plans {
		require(plan.IndexedMSMonth >= 0 && plan.LLMTokensDay >= 0, "limits.plans."+name)
	}
	if role == RoleAPI {
		require(c.Server.Addr != "", "server.addr")
		for _, origin := range c.Server.CORSOrigins {
			require(httpURL(origin), "server.cors_origins")
		}
		require(httpURL(c.Auth.Issuer) && httpURL(c.Auth.JWKSURL) && c.Auth.Audience != "", "auth.issuer / auth.jwks_url / auth.audience")
	} else {
		require(c.Worker.CPUConcurrency > 0 && c.Worker.GPUConcurrency > 0, "worker concurrency")
		require(c.Worker.WorkspaceGPULimit > 0 && c.Worker.WorkspaceGPULimit < c.Worker.GPUConcurrency, "worker.workspace_gpu_limit")
		require(c.Worker.MaxRetry >= 0 && c.Worker.ReconcileEvery > 0, "worker retry / reconciliation")
		require(c.ML.EmbedBatch > 0 && c.ML.TranscribeChunkMS > 0 && c.ML.TranscribeOverlapMS >= 0 && c.ML.TranscribeOverlapMS < c.ML.TranscribeChunkMS && c.ML.TranscribeTimeout > 0, "ml transcription / embed limits")
		require(c.Media.NormalizedVideoMaxHeight > 0 && c.Media.NormalizedVideoGOPSeconds > 0 && c.Media.ThumbWidth > 0 && c.Media.SpriteCols > 0 && c.Media.SpriteRows > 0 && c.Media.TmpDir != "", "media")
	}
	require(c.Import.YTDLPPath != "" && c.Import.MaxHeight > 0 && c.Import.MaxDurationMS > 0 && c.Import.MaxBytes > 0 && c.Import.Timeout > 0, "import limits")
	for name, p := range c.Import.Platforms {
		require(len(p.Hosts) > 0 && len(p.Extractors) > 0 && p.MaxConcurrency > 0, "import.platforms."+name)
	}
	return errors.Join(errs...)
}

func httpURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}
