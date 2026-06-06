package service

import (
	"context"
	"encoding/json"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
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

func (c skillPermissionChecker) ApprovedCredentials(
	ctx context.Context,
	tenantID uint64,
	skillName string,
) (types.JSON, error) {
	credential, err := c.repo.GetTenantSkillCredentialByName(ctx, tenantID, skillName)
	if err != nil {
		return nil, err
	}
	credentialMap, err := credential.Credentials.Map()
	if err != nil {
		return nil, err
	}
	for key, raw := range credentialMap {
		value, ok := raw.(string)
		if !ok {
			continue
		}
		if plain, ok := utils.DecryptStoredSecretLenient(value); ok {
			credentialMap[key] = plain
		}
	}
	raw, err := json.Marshal(credentialMap)
	if err != nil {
		return nil, err
	}
	return types.JSON(raw), nil
}

func (c skillPermissionChecker) ApprovedMCPBindings(
	ctx context.Context,
	tenantID uint64,
	skillName string,
) (types.JSON, error) {
	bindings, err := c.repo.ListTenantSkillMCPBindingsByName(ctx, tenantID, skillName)
	if err != nil {
		return nil, err
	}
	bindingMap := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		bindingMap[binding.MCPName] = binding.ServiceID
	}
	raw, err := json.Marshal(bindingMap)
	if err != nil {
		return nil, err
	}
	return types.JSON(raw), nil
}
