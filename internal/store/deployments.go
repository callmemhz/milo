package store

import (
	"context"
	"time"

	"github.com/callmemhz/milo/internal/store/sqlcgen"
)

type Deployment = sqlcgen.Deployment

const (
	DeployPending    = "pending"
	DeployDeploying  = "deploying"
	DeploySucceeded  = "succeeded"
	DeployFailed     = "failed"
	DeploySuperseded = "superseded"
	DeployCancelled  = "cancelled"
)

func (s *Store) CreateDeployment(ctx context.Context, appID int64, digest, ref, commit, gitref string, triggeredBy int64) (Deployment, error) {
	var commitPtr, refPtr *string
	if commit != "" {
		commitPtr = &commit
	}
	if gitref != "" {
		refPtr = &gitref
	}
	return s.Q.CreateDeployment(ctx, sqlcgen.CreateDeploymentParams{
		AppID:       appID,
		ImageDigest: digest,
		ImageRef:    ref,
		CommitSha:   commitPtr,
		Ref:         refPtr,
		Status:      DeployPending,
		TriggeredBy: triggeredBy,
		CreatedAt:   time.Now().UTC(),
	})
}

func (s *Store) GetDeployment(ctx context.Context, id int64) (Deployment, error) {
	return s.Q.GetDeployment(ctx, id)
}

func (s *Store) ListDeploymentsForApp(ctx context.Context, appID int64, limit, offset int64) ([]Deployment, error) {
	return s.Q.ListDeploymentsForApp(ctx, sqlcgen.ListDeploymentsForAppParams{AppID: appID, Limit: limit, Offset: offset})
}

func (s *Store) UpdateDeploymentStatus(ctx context.Context, id int64, status, failureReason, containerName string) error {
	now := time.Now().UTC()
	var failPtr, namePtr *string
	if failureReason != "" {
		failPtr = &failureReason
	}
	if containerName != "" {
		namePtr = &containerName
	}
	return s.Q.UpdateDeploymentStatus(ctx, sqlcgen.UpdateDeploymentStatusParams{
		Status:        status,
		FailureReason: failPtr,
		ContainerName: namePtr,
		FinishedAt:    &now,
		ID:            id,
	})
}

func (s *Store) ListInflightDeployments(ctx context.Context) ([]Deployment, error) {
	return s.Q.ListInflightDeployments(ctx)
}
