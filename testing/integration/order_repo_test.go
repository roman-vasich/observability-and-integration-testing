//go:build integration

package integration

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/roman-vasich/observability-and-integration-testing/testing/setup"
)

func TestOrderRepo_Integration(t *testing.T) {
	requireDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	pg, err := setup.StartPostgres(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = pg.Close(context.Background())
	})

	repo := PostgresOrderRepo{db: pg.DB}
	require.NoError(t, repo.initSchema(ctx))

	created := Order{
		ID:         "order-123",
		CustomerID: "customer-1",
		Status:     "pending",
		TotalCents: 1250,
		Note:       "first order",
	}

	require.NoError(t, repo.CreateMut(ctx, created))

	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)

	nextStatus := "paid"
	require.NoError(t, repo.UpdateMut(ctx, created.ID, OrderPatch{Status: &nextStatus}))

	updated, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "paid", updated.Status)
	assert.Equal(t, created.CustomerID, updated.CustomerID)
	assert.Equal(t, created.TotalCents, updated.TotalCents)
	assert.Equal(t, created.Note, updated.Note)
}

func requireDocker(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		t.Skipf("docker unavailable, skipping integration test: %v", err)
	}
}
