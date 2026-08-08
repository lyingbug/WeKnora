package container

import (
	"context"
	"os"
	"strings"

	"go.uber.org/dig"

	"github.com/Tencent/WeKnora/internal/application/repository"
	memorysvc "github.com/Tencent/WeKnora/internal/application/service/memory"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// registerMemoryComponents wires the long-term memory subsystem.
//
// Kept in its own file because the subsystem has to be assembled in a specific
// order and needs one hand-written provider: the settings service takes the
// deployment description, which is the only place in the codebase that is
// allowed to look at the product edition, and it must not be confused with the
// infrastructure facts (SQLite, no Redis) that everything else keys off.
func registerMemoryComponents(c *dig.Container, redisAvailable bool) {
	must(c.Provide(repository.NewMemorySpaceRepository))
	must(c.Provide(repository.NewMemoryPageRepository))
	must(c.Provide(repository.NewMemoryNoteRepository))
	must(c.Provide(repository.NewMemoryAnchorRepository))

	must(c.Provide(func(db *gorm.DB) memorysvc.Deployment {
		dialect := ""
		if db != nil && db.Dialector != nil {
			dialect = db.Dialector.Name()
		}
		return memorysvc.Deployment{
			// Edition is only consulted for the shared-space capability. Every
			// behavioural decision keys off the two facts below instead, so a
			// standard binary running the Lite profile behaves like Lite.
			Edition:  strings.ToLower(handler.Edition),
			HasRedis: redisAvailable,
			Dialect:  dialect,
		}
	}))

	must(c.Provide(func(
		deployment memorysvc.Deployment,
		tenants interfaces.TenantService,
		users interfaces.UserService,
		agents interfaces.CustomAgentService,
		spaces interfaces.MemorySpaceRepository,
	) interfaces.MemorySettingsService {
		return memorysvc.NewSettingsService(deployment, tenants, users, agents, spaces)
	}))

	// The concrete service is provided as well as the interface: the writer
	// reuses its page-write path, which is intentionally not part of the public
	// interface because nothing outside the subsystem should call it.
	must(c.Provide(memorysvc.NewService))
	must(c.Provide(func(s *memorysvc.Service) interfaces.MemoryService { return s }))

	must(c.Provide(memorysvc.NewRecallService))
	must(c.Provide(memorysvc.NewWriterService))

	must(c.Provide(memorysvc.NewTaskHandler))
	must(c.Provide(handler.NewMemoryHandler))
}

// memoryDecayScheduleEnabled reports whether this process should run the daily
// decay sweep. Disabled by default in tests via the env guard so a unit run
// never spawns a background sweeper.
func memoryDecayScheduleEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("MEMORY_DISABLE_DECAY_SWEEP")), "true")
}

// startMemoryDecaySweep runs the retention sweep daily, beginning a few minutes
// after boot so the first pass does not contend with startup work.
//
// Scheduled here rather than through asynq's periodic scheduler because it must
// also run on Lite, where there is no Redis and therefore no scheduler; a
// goroutine with a ticker behaves identically in both deployments.
func startMemoryDecaySweep(ctx context.Context, writer interfaces.MemoryWriterService) {
	if !memoryDecayScheduleEnabled() {
		return
	}
	go memorysvc.RunDecaySchedule(ctx, writer)
}
