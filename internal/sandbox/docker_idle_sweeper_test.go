package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

// newSweeperFixture wires a sweeper over a fake daemon holding the given
// containers, with the activity marker of each one set from markers.
func newSweeperFixture(
	t *testing.T,
	ttl time.Duration,
	now time.Time,
	containers []container.Summary,
) (*dockerIdleSweeper, *fakeDockerEngine) {
	t.Helper()
	engine := newFakeDockerEngine()
	engine.list = containers
	docker := newTestDockerClient(t, engine)
	sweeper := newDockerIdleSweeper(docker, ttl)
	sweeper.now = func() time.Time { return now }
	return sweeper, engine
}

func TestDockerIdleSweeperReclaimsOnlyIdleContainers(t *testing.T) {
	now := time.Now().UTC()
	sweeper, engine := newSweeperFixture(t, 30*time.Minute, now, []container.Summary{
		{ID: "busy", State: "running", Created: now.Add(-4 * time.Hour).Unix(),
			Labels: map[string]string{dockerManagedLabel: "true"}},
		{ID: "idle", State: "running", Created: now.Add(-4 * time.Hour).Unix(),
			Labels: map[string]string{dockerManagedLabel: "true"}},
	})
	// The marker is what every exec touches; its mtime is the only record of
	// when a sandbox was last used.
	engine.statResult[dockerActivityMarker] = container.PathStat{Mtime: now.Add(-2 * time.Minute)}

	reclaimed, err := sweeper.sweep(context.Background())
	require.NoError(t, err)
	require.Zero(t, reclaimed, "a recently used sandbox must survive")
	require.Empty(t, engine.removed)

	engine.statResult[dockerActivityMarker] = container.PathStat{Mtime: now.Add(-90 * time.Minute)}
	reclaimed, err = sweeper.sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, reclaimed)
	require.ElementsMatch(t, []string{"busy", "idle"}, engine.removed)
}

// A sandbox that has never executed anything has no marker, so its creation
// time decides. Without this a freshly created container would look infinitely
// idle and be deleted out from under the session that just created it.
func TestDockerIdleSweeperFallsBackToCreationTime(t *testing.T) {
	now := time.Now().UTC()
	sweeper, engine := newSweeperFixture(t, 30*time.Minute, now, []container.Summary{
		{ID: "fresh", State: "running", Created: now.Add(-time.Minute).Unix(),
			Labels: map[string]string{dockerManagedLabel: "true"}},
	})

	reclaimed, err := sweeper.sweep(context.Background())
	require.NoError(t, err)
	require.Zero(t, reclaimed)
	require.Empty(t, engine.removed)
}

// Each container carries the TTL it was created with, so a sweep triggered by
// one workspace config cannot apply its own TTL to another config's sandboxes.
func TestDockerIdleSweeperHonoursPerContainerTTL(t *testing.T) {
	now := time.Now().UTC()
	sweeper, engine := newSweeperFixture(t, time.Minute, now, []container.Summary{
		{ID: "long-ttl", State: "running", Created: now.Add(-2 * time.Hour).Unix(),
			Labels: map[string]string{
				dockerManagedLabel: "true",
				dockerIdleTTLLabel: "86400",
			}},
	})
	engine.statResult[dockerActivityMarker] = container.PathStat{Mtime: now.Add(-2 * time.Hour)}

	reclaimed, err := sweeper.sweep(context.Background())
	require.NoError(t, err)
	require.Zero(t, reclaimed)
	require.Empty(t, engine.removed)
}

// The sweep runs off the back of ordinary requests, so it must not turn into a
// list-plus-stat storm when a workspace is busy.
func TestDockerIdleSweeperThrottlesPerDaemon(t *testing.T) {
	now := time.Now().UTC()
	sweeper, _ := newSweeperFixture(t, time.Minute, now, nil)
	dockerSweepThrottle.mu.Lock()
	delete(dockerSweepThrottle.last, sweeper.client.settings.Endpoint.key())
	dockerSweepThrottle.mu.Unlock()

	require.True(t, sweeper.claimSweep(), "the first caller sweeps")
	require.False(t, sweeper.claimSweep(), "a second caller in the same minute does not")

	sweeper.now = func() time.Time { return now.Add(2 * dockerSweepMinInterval) }
	require.True(t, sweeper.claimSweep(), "the throttle expires")
}

// A container the daemon refuses to delete must not abort the pass: the rest
// of the idle set still has to be reclaimed.
func TestDockerIdleSweeperContinuesAfterDeleteFailure(t *testing.T) {
	now := time.Now().UTC()
	sweeper, engine := newSweeperFixture(t, time.Minute, now, []container.Summary{
		{ID: "wedged", State: "running", Created: now.Add(-2 * time.Hour).Unix(),
			Labels: map[string]string{dockerManagedLabel: "true"}},
		{ID: "reclaimable", State: "running", Created: now.Add(-2 * time.Hour).Unix(),
			Labels: map[string]string{dockerManagedLabel: "true"}},
	})
	engine.removeErr = context.DeadlineExceeded

	reclaimed, err := sweeper.sweep(context.Background())
	require.NoError(t, err)
	require.Zero(t, reclaimed)
	require.Len(t, engine.removed, 2, "every candidate is attempted")
}
