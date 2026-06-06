<template>
  <div class="skill-settings">
    <div class="section-header">
      <h2>Skills</h2>
      <p class="section-description">管理租户 Skill 安装、权限、凭据和 MCP 绑定。</p>
    </div>

    <div class="install-panel">
      <t-tabs v-model="installMode" size="medium">
        <t-tab-panel value="local" label="本地包">
          <div class="install-row">
            <t-input v-model="localPackagePath" placeholder="package path" clearable />
            <t-button theme="default" variant="outline" :loading="previewing" @click="handlePreviewLocal">
              <template #icon><t-icon name="browse" /></template>
              预览
            </t-button>
            <t-button theme="primary" :loading="installing" @click="handleInstallLocal">
              <template #icon><t-icon name="download" /></template>
              安装
            </t-button>
          </div>
        </t-tab-panel>
        <t-tab-panel value="hub" label="Skill Hub">
          <div class="install-row">
            <t-input v-model="hubSourceUrl" placeholder="https://hub.example.com/packages/skill.zip" clearable />
            <t-button theme="default" variant="outline" :loading="previewing" @click="handlePreviewHub">
              <template #icon><t-icon name="browse" /></template>
              预览
            </t-button>
            <t-button theme="primary" :loading="installing" @click="handleInstallHub">
              <template #icon><t-icon name="download" /></template>
              安装
            </t-button>
          </div>
        </t-tab-panel>
      </t-tabs>

      <div v-if="preview" class="preview-box">
        <div class="preview-title">
          <span>{{ preview.name }}@{{ preview.version }}</span>
          <t-tag size="small" theme="primary" variant="light">{{ preview.source_type }}</t-tag>
        </div>
        <div class="preview-desc">{{ preview.description }}</div>
        <t-textarea
          v-model="approvedPermissionsText"
          class="json-editor"
          autosize
          placeholder="approved_permissions JSON"
        />
      </div>
    </div>

    <div class="settings-toolbar">
      <div class="toolbar-info">
        <h3>已安装 Skills</h3>
        <p>{{ installedSkills.length }} 个租户级安装记录</p>
      </div>
      <t-button variant="outline" size="small" :loading="loading" @click="loadInstalledSkills">
        <template #icon><t-icon name="refresh" /></template>
        刷新
      </t-button>
    </div>

    <div v-if="loading" class="loading-container">
      <t-loading text="加载中" />
    </div>
    <t-empty v-else-if="installedSkills.length === 0" description="暂无已安装 Skill" />
    <div v-else class="skill-list">
      <div v-for="skill in installedSkills" :key="skill.id" class="skill-card">
        <div class="skill-card__main">
          <div class="skill-card__title">
            <h3 :title="skill.name">{{ skill.name }}</h3>
            <t-tag size="small" variant="light">{{ skill.version }}</t-tag>
            <t-tag size="small" :theme="skill.source_type === 'hub' ? 'success' : 'primary'" variant="light">
              {{ skill.source_type }}
            </t-tag>
          </div>
          <p class="skill-card__desc">{{ skill.description }}</p>
          <div class="skill-card__meta">
            <span>{{ skill.id }}</span>
            <span v-if="skill.installed_by">by {{ skill.installed_by }}</span>
          </div>
        </div>
        <div class="skill-card__actions">
          <t-switch :model-value="skill.enabled" @change="(value: boolean) => handleToggle(skill, Boolean(value))" />
          <t-button size="small" variant="outline" @click="openConfig(skill)">
            <template #icon><t-icon name="setting" /></template>
            配置
          </t-button>
        </div>
      </div>
    </div>

    <t-dialog
      v-model:visible="configVisible"
      :header="currentSkill ? `${currentSkill.name} 配置` : 'Skill 配置'"
      width="720px"
      attach="body"
      @confirm="handleSaveConfig"
    >
      <div class="config-grid">
        <div>
          <div class="field-label">Credentials JSON</div>
          <t-textarea v-model="credentialsText" autosize class="json-editor" placeholder='{"API_KEY":"secret"}' />
        </div>
        <div>
          <div class="field-label">MCP Bindings JSON</div>
          <t-textarea v-model="mcpBindingsText" autosize class="json-editor" placeholder='{"github":"mcp-service-id"}' />
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  installHubSkillPackage,
  installLocalSkillPackage,
  listInstalledSkills,
  previewHubSkillPackage,
  previewLocalSkillPackage,
  updateSkillCredentials,
  updateSkillEnabled,
  updateSkillMCPBindings,
  type InstalledSkill,
  type SkillPackagePreview,
} from '@/api/skill'

const loading = ref(false)
const previewing = ref(false)
const installing = ref(false)
const installMode = ref<'local' | 'hub'>('local')
const localPackagePath = ref('')
const hubSourceUrl = ref('')
const preview = ref<SkillPackagePreview | null>(null)
const approvedPermissionsText = ref('{}')
const installedSkills = ref<InstalledSkill[]>([])
const configVisible = ref(false)
const currentSkill = ref<InstalledSkill | null>(null)
const credentialsText = ref('{}')
const mcpBindingsText = ref('{}')

const parseJSONObject = (value: string, label: string) => {
  try {
    const parsed = JSON.parse(value || '{}')
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
      throw new Error(`${label} must be an object`)
    }
    return parsed
  } catch (error: any) {
    throw new Error(`${label}: ${error?.message || 'invalid JSON'}`)
  }
}

const loadInstalledSkills = async () => {
  loading.value = true
  try {
    installedSkills.value = await listInstalledSkills()
  } catch (error) {
    MessagePlugin.error('加载 Skills 失败')
  } finally {
    loading.value = false
  }
}

const setPreview = (data: SkillPackagePreview) => {
  preview.value = data
  approvedPermissionsText.value = JSON.stringify(data.requested_permissions || {}, null, 2)
}

const handlePreviewLocal = async () => {
  if (!localPackagePath.value.trim()) return MessagePlugin.warning('请输入本地包路径')
  previewing.value = true
  try {
    setPreview(await previewLocalSkillPackage(localPackagePath.value.trim()))
  } catch (error: any) {
    MessagePlugin.error(error?.message || '预览失败')
  } finally {
    previewing.value = false
  }
}

const handlePreviewHub = async () => {
  if (!hubSourceUrl.value.trim()) return MessagePlugin.warning('请输入 Skill Hub URL')
  previewing.value = true
  try {
    setPreview(await previewHubSkillPackage(hubSourceUrl.value.trim()))
  } catch (error: any) {
    MessagePlugin.error(error?.message || '预览失败')
  } finally {
    previewing.value = false
  }
}

const handleInstallLocal = async () => {
  if (!localPackagePath.value.trim()) return MessagePlugin.warning('请输入本地包路径')
  installing.value = true
  try {
    await installLocalSkillPackage(localPackagePath.value.trim(), parseJSONObject(approvedPermissionsText.value, 'approved_permissions'))
    MessagePlugin.success('安装成功')
    await loadInstalledSkills()
  } catch (error: any) {
    MessagePlugin.error(error?.message || '安装失败')
  } finally {
    installing.value = false
  }
}

const handleInstallHub = async () => {
  if (!hubSourceUrl.value.trim()) return MessagePlugin.warning('请输入 Skill Hub URL')
  installing.value = true
  try {
    await installHubSkillPackage(hubSourceUrl.value.trim(), parseJSONObject(approvedPermissionsText.value, 'approved_permissions'))
    MessagePlugin.success('安装成功')
    await loadInstalledSkills()
  } catch (error: any) {
    MessagePlugin.error(error?.message || '安装失败')
  } finally {
    installing.value = false
  }
}

const handleToggle = async (skill: InstalledSkill, enabled: boolean) => {
  try {
    await updateSkillEnabled(skill.id, enabled)
    skill.enabled = enabled
    MessagePlugin.success('已更新')
  } catch (error: any) {
    MessagePlugin.error(error?.message || '更新失败')
  }
}

const openConfig = (skill: InstalledSkill) => {
  currentSkill.value = skill
  credentialsText.value = '{}'
  mcpBindingsText.value = '{}'
  configVisible.value = true
}

const handleSaveConfig = async () => {
  if (!currentSkill.value) return
  try {
    await updateSkillCredentials(currentSkill.value.id, parseJSONObject(credentialsText.value, 'credentials'))
    await updateSkillMCPBindings(currentSkill.value.id, parseJSONObject(mcpBindingsText.value, 'mcp bindings'))
    MessagePlugin.success('配置已保存')
    configVisible.value = false
  } catch (error: any) {
    MessagePlugin.error(error?.message || '保存失败')
  }
}

onMounted(loadInstalledSkills)
</script>

<style scoped>
.skill-settings {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.section-header h2,
.toolbar-info h3 {
  margin: 0;
}

.section-description,
.toolbar-info p,
.skill-card__desc,
.skill-card__meta,
.preview-desc {
  color: var(--td-text-color-secondary);
  margin: 4px 0 0;
}

.install-panel {
  border: 1px solid var(--td-border-level-1-color);
  border-radius: 8px;
  padding: 16px;
  background: var(--td-bg-color-container);
}

.install-row,
.settings-toolbar,
.skill-card,
.skill-card__title,
.skill-card__actions,
.preview-title {
  display: flex;
  align-items: center;
  gap: 12px;
}

.install-row {
  margin-top: 12px;
}

.settings-toolbar,
.skill-card {
  justify-content: space-between;
}

.preview-box {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--td-border-level-1-color);
}

.preview-title {
  font-weight: 600;
}

.json-editor {
  margin-top: 10px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.skill-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.skill-card {
  border: 1px solid var(--td-border-level-1-color);
  border-radius: 8px;
  padding: 14px;
  background: var(--td-bg-color-container);
}

.skill-card__main {
  min-width: 0;
}

.skill-card__title h3 {
  margin: 0;
  font-size: 15px;
}

.skill-card__meta {
  display: flex;
  gap: 12px;
  font-size: 12px;
}

.config-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.field-label {
  font-weight: 600;
}
</style>
