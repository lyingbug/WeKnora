import { get, patch, post, put } from "../../utils/request";

// Skill信息
export interface SkillInfo {
  name: string;
  description: string;
}

export interface InstalledSkill {
  id: string;
  name: string;
  version: string;
  description: string;
  source_type: string;
  enabled: boolean;
  installed_by?: string;
  approved_permissions?: Record<string, any>;
  is_builtin?: boolean;
}

export interface SkillPackagePreview {
  name: string;
  version: string;
  description: string;
  source_type: string;
  source_uri: string;
  digest: string;
  manifest: Record<string, any>;
  requested_permissions: Record<string, any>;
}

// 获取预装Skills列表；skills_available 为 false 表示沙箱未启用，前端应隐藏/禁用 Skills 配置
export function listSkills() {
  return get('/api/v1/skills') as Promise<{ data: SkillInfo[]; skills_available?: boolean }>;
}

export async function listInstalledSkills() {
  const res = await get('/api/v1/skills/installed') as { data: InstalledSkill[] };
  return res.data || [];
}

export async function previewLocalSkillPackage(packagePath: string) {
  const res = await post('/api/v1/skills/preview-local', { package_path: packagePath }) as { data: SkillPackagePreview };
  return res.data;
}

export async function previewHubSkillPackage(sourceUrl: string) {
  const res = await post('/api/v1/skills/preview-hub', { source_url: sourceUrl }) as { data: SkillPackagePreview };
  return res.data;
}

export async function installLocalSkillPackage(packagePath: string, approvedPermissions: Record<string, any>) {
  const res = await post('/api/v1/skills/install-local', {
    package_path: packagePath,
    approved_permissions: approvedPermissions,
  }) as { data: InstalledSkill };
  return res.data;
}

export async function installHubSkillPackage(sourceUrl: string, approvedPermissions: Record<string, any>) {
  const res = await post('/api/v1/skills/install-hub', {
    source_url: sourceUrl,
    approved_permissions: approvedPermissions,
  }) as { data: InstalledSkill };
  return res.data;
}

export function updateSkillEnabled(skillId: string, enabled: boolean) {
  return patch(`/api/v1/skills/${encodeURIComponent(skillId)}`, { enabled });
}

export function updateSkillCredentials(skillId: string, credentials: Record<string, string>) {
  return put(`/api/v1/skills/${encodeURIComponent(skillId)}/credentials`, { credentials });
}

export function updateSkillMCPBindings(skillId: string, bindings: Record<string, string>) {
  return put(`/api/v1/skills/${encodeURIComponent(skillId)}/mcp-bindings`, { bindings });
}
