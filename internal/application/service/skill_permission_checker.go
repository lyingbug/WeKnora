package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type skillPermissionChecker struct {
	repo interfaces.SkillRepository
}

func (c skillPermissionChecker) ApprovedPermissions(
	ctx context.Context,
	tenantID uint64,
	skillName string,
) (types.JSON, error) {
	install, err := c.repo.GetTenantSkillInstallEntryByName(ctx, tenantID, skillName)
	if err != nil {
		return nil, err
	}
	return install.ApprovedPermissions, nil
}
